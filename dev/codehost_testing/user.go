package codehost_scenario

import (
	"context"

	"github.com/google/go-github/v53/github"
)

// User represents a GitHub user in the scenario.
type User struct {
	s    *GithubScenario
	name string
}

// Get returns the corresponding GitHub user object that was created by the `CreateUser`
//
// This method will only return a User if the Scenario that created it has been applied otherwise
// it will panic.
func (u *User) Get(ctx context.Context) (*github.User, error) {
	if u.s.IsApplied() {
		return u.get(ctx)
	}
	panic("cannot retrieve user before scenario is applied")
}

// get retrieves the GitHub user without panicking if not applied. It is meant as an
// internal helper method while actions are getting applied.
func (u *User) get(ctx context.Context) (*github.User, error) {
	return u.s.client.GetUser(ctx, u.name)
}
