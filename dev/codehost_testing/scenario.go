package codehost_testing

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sourcegraph/sourcegraph/dev/codehost_testing/config"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// action represents an action to perform against a codehost. Each action should provide
// a name and functions to apply and optionally teardown the action. Where teardown destroys
// the resource created in apply.
type action struct {
	name     string
	apply    func(context.Context) error
	teardown func(context.Context) error
}

// Scenario is an interface for executing a sequence of actions in a test scenario. Actions can be added
// by the relevant struct implementing the interface.
//
// The methods of the interface have the following intentions:
// Apply: should apply the actions that are part of the interface
// Teardown: should remove or undo the actions applied by Apply
// Plan: should return a human-readable string describing the actions that will be performed
type Scenario interface {
	append(a ...*action)
	Plan() string
	Apply(ctx context.Context) error
	Teardown(ctx context.Context) error
}

// GithubScenario implements the Scenario interface for testing GitHub functionality. At its base GithubScenario
// provides two top level methods to create GitHub resources namely:
// * create GitHub Organization, which returns a codehost_scenario Org
// * create a GitHub User, which returns a codehost_scenario User
//
// Further resources can be created by calling methods on the returned Org or User. For instance, since a repository
// is tied to an organization, one can call org.CreateRepo, which will add an action for a repo to be created in the
// organization.
//
// Calling any action creating method does not immediately create the resource in GitHub. Instead a action is added
// the list of actions contained in this scenario. Only once Apply() has been called on the Scenario itself will
// the resources be created on GitHub.
//
// Once Apply() is called, all the corresponding resources should be realized on GitHub. To fetch the corresponding
// GitHub resources once can call Get() on the resources.
type GithubScenario struct {
	id               string
	t                *testing.T
	client           *GitHubClient
	actions          []*action
	reporter         Reporter
	appliedActionIdx int
}

var _ Scenario = (*GithubScenario)(nil)

// NewGithubScenario creates a new GithubScenario instance. A base64 ID will be generated to identify this scenario.
// This ID will also be used to uniquely identify any resources created as part of this scenario.
//
// By default a GithubScenario is created with a NoopReporter. To have more verbose output, call Verbose() on the scenario,
// and to reduce the output, call Quiet().
func NewGithubScenario(ctx context.Context, t *testing.T, cfg config.Config) (*GithubScenario, error) {
	client, err := NewGitHubClient(ctx, t, cfg.GitHub)
	if err != nil {
		return nil, err
	}
	uid := []byte(uuid.NewString())
	id := base64.RawStdEncoding.EncodeToString(uid[:])[:10]
	return &GithubScenario{
		id:       id,
		t:        t,
		client:   client,
		actions:  make([]*action, 0),
		reporter: NoopReporter{},
	}, nil
}

// Verbose sets the reporter to ConsoleReporter to enable verbose output
func (s *GithubScenario) Verbose() {
	s.reporter = &ConsoleReporter{}
}

// Quiet sets the reporter to a no-op reporter to reduce output
func (s *GithubScenario) Quiet() {
	s.reporter = NoopReporter{}
}

func (s *GithubScenario) append(actions ...*action) {
	s.actions = append(s.actions, actions...)
}

// Plan returns a string describing the actions that will be performed
func (s *GithubScenario) Plan() string {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "Scenario %q\n", s.id)
	sb.WriteString("== Setup ==\n")
	for _, action := range s.actions {
		fmt.Fprintf(sb, "- %s\n", action.name)
	}
	sb.WriteString("== Teardown ==\n")
	for _, action := range reverse(s.actions) {
		if action.teardown == nil {
			continue
		}
		fmt.Fprintf(sb, "- %s\n", action.name)
	}
	return sb.String()
}

// IsApplied returns whether Apply has already been called on this scenario. If more actions
// have been added since the last Apply(), it will return false.
func (s *GithubScenario) IsApplied() bool {
	return s.appliedActionIdx >= len(s.actions)
}

// Apply performs all the actions that have been added to this scenario sequentially in the order they were added.
// Furthemore cleanup function is registered so Teardown is called even if Apply fails to make sure we cleanup any
// left over resources due to a half applied scenario.
//
// Note that calling Apply more than once and with no new actions added, will result in an error be returned. This
// is done since duplicate resources cannot be created.
//
// Finally, if any action fails, no further actions will be executed and this method will return with the error
func (s *GithubScenario) Apply(ctx context.Context) error {
	s.t.Helper()
	s.t.Cleanup(func() { s.Teardown(ctx) })
	var errs errors.MultiError
	setup := s.actions
	failFast := true

	if s.appliedActionIdx >= len(s.actions) {
		return errors.New("all actions already applied")
	}

	start := time.Now()
	for i, action := range setup {
		now := time.Now().UTC()

		var err error
		if s.appliedActionIdx <= i {
			s.reporter.Writef("(Setup) Applying [%-50s] ", action.name)
			err = action.apply(ctx)
			s.appliedActionIdx++
		} else {
			s.reporter.Writef("(Setup) Skipping [%-50s]\n", action.name)
			continue
		}

		duration := time.Now().UTC().Sub(now)
		if err != nil {
			s.reporter.Writef("FAILED (%s)\n", duration.String())
			if failFast {
				return err
			}
			errs = errors.Append(errs, err)
		} else {
			s.reporter.Writef("SUCCESS (%s)\n", duration.String())
		}
	}

	s.reporter.Writef("Setup complete in %s\n\n", time.Now().UTC().Sub(start))
	return errs
}

// Teardown cleans up any resources created by Apply. This method is automatically registerd with *testing.Cleanup to
// cleanup resources, so generally it would not have to be called explicitly.
//
// Teardown iterates through the scenario actions in reverse order, calling teardown on each action. If a action
// has a nil teardown function it will be skipped. Teardown does not stop iterating when an action returns with an error,
// instead the error is accumulated and the next teardown action is executed.
//
// Note that Teardown is not idempotent. Multiple calls will result in failures.
func (s *GithubScenario) Teardown(ctx context.Context) error {
	s.t.Helper()
	var errs errors.MultiError
	teardown := reverse(s.actions)
	failFast := false

	start := time.Now()
	for _, action := range teardown {
		s.appliedActionIdx--
		if action.teardown == nil {
			continue
		}
		now := time.Now().UTC()

		s.reporter.Writef("(Teardown) Applying [%-50s] ", action.name)
		err := action.teardown(ctx)
		duration := time.Now().UTC().Sub(now)

		if err != nil {
			s.reporter.Writef("FAILED (%s)\n", duration.String())
			if failFast {
				return err
			}
			errs = errors.Append(errs, err)
		} else {
			s.reporter.Writef("SUCCESS (%s)\n", duration.String())
		}
	}
	if s.appliedActionIdx < 0 {
		s.t.Logf("scenario applied action Idx went negative. This is almost certainly a bug")
		s.appliedActionIdx = 0
	}
	s.reporter.Writef("Teardown complete in %s\n", time.Now().UTC().Sub(start))
	return errs
}

func (s *GithubScenario) CreateOrg(name string) *Org {
	baseOrg := &Org{
		s:    s,
		name: name,
	}

	createOrg := &action{
		name: "org:create:" + name,
		apply: func(ctx context.Context) error {
			orgName := fmt.Sprintf("org-%s-%s", name, s.id)
			org, err := s.client.CreateOrg(ctx, orgName)
			if err != nil {
				return err
			}
			baseOrg.name = org.GetLogin()
			return nil
		},
		teardown: func(context.Context) error {
			host := baseOrg.s.client.cfg.URL
			deleteURL := fmt.Sprintf("%s/organizations/%s/settings/profile", host, baseOrg.name)
			fmt.Printf("Visit %q to delete the org\n", deleteURL)
			return nil
		},
	}

	s.append(createOrg)
	return baseOrg
}

// CreateUser adds an action to the scenario that will create a GitHub user with the given name. The username of the
// user will have the following format `user-{name}-{scenario id}` and email `test-user-e2e@sourcegraph.com`.
func (s *GithubScenario) CreateUser(name string) *User {
	baseUser := &User{
		s:    s,
		name: name,
	}

	createUser := &action{
		name: "user:create:" + name,
		apply: func(ctx context.Context) error {
			name := fmt.Sprintf("user-%s-%s", name, s.id)
			email := "test-user-e2e@sourcegraph.com"
			user, err := s.client.CreateUser(ctx, name, email)
			if err != nil {
				return err
			}

			baseUser.name = user.GetLogin()
			return nil
		},
		teardown: func(ctx context.Context) error {
			return s.client.DeleteUser(ctx, baseUser.name)
		},
	}

	s.append(createUser)
	return baseUser
}

// GetAdmin returns a User representing the GitHub admin user configured in the client.
//
// NOTE: this method does not actually add an explicit action to the scenario, but will still
// require that the scenario has been applied before the admin user can be retrieved - even though
// it is not strictly required as the Admin already exists.
func (s *GithubScenario) GetAdmin() *User {
	baseUser := &User{
		s:    s,
		name: s.client.cfg.AdminUser,
	}

	return baseUser
}
