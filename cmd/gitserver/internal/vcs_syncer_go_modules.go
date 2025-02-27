package internal

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/sourcegraph/log"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"

	"github.com/sourcegraph/sourcegraph/internal/api"

	"github.com/sourcegraph/sourcegraph/internal/codeintel/dependencies"
	"github.com/sourcegraph/sourcegraph/internal/conf/reposource"
	"github.com/sourcegraph/sourcegraph/internal/extsvc/gomodproxy"
	"github.com/sourcegraph/sourcegraph/internal/unpack"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/schema"
)

func NewGoModulesSyncer(
	connection *schema.GoModulesConnection,
	svc *dependencies.Service,
	client *gomodproxy.Client,
) VCSSyncer {
	placeholder, err := reposource.ParseGoVersionedPackage("sourcegraph.com/placeholder@v0.0.0")
	if err != nil {
		panic(fmt.Sprintf("expected placeholder dependency to parse but got %v", err))
	}

	return &vcsPackagesSyncer{
		logger:      log.Scoped("GoModulesSyncer", "sync Go modules"),
		typ:         "go_modules",
		scheme:      dependencies.GoPackagesScheme,
		placeholder: placeholder,
		svc:         svc,
		configDeps:  connection.Dependencies,
		source:      &goModulesSyncer{client: client},
	}
}

type goModulesSyncer struct {
	client *gomodproxy.Client
}

func (s goModulesSyncer) ParseVersionedPackageFromNameAndVersion(name reposource.PackageName, version string) (reposource.VersionedPackage, error) {
	return reposource.ParseGoVersionedPackage(string(name) + "@" + version)
}

func (goModulesSyncer) ParseVersionedPackageFromConfiguration(dep string) (reposource.VersionedPackage, error) {
	return reposource.ParseGoVersionedPackage(dep)
}

func (goModulesSyncer) ParsePackageFromName(name reposource.PackageName) (reposource.Package, error) {
	return reposource.ParseGoDependencyFromName(name)
}

func (goModulesSyncer) ParsePackageFromRepoName(repoName api.RepoName) (reposource.Package, error) {
	return reposource.ParseGoDependencyFromRepoName(repoName)
}

func (s *goModulesSyncer) Download(ctx context.Context, dir string, dep reposource.VersionedPackage) error {
	zipBytes, err := s.client.GetZip(ctx, dep.PackageSyntax(), dep.PackageVersion())
	if err != nil {
		return errors.Wrap(err, "get zip")
	}

	mod := dep.(*reposource.GoVersionedPackage).Module
	if err = unzip(mod, zipBytes, dir); err != nil {
		return errors.Wrap(err, "failed to unzip go module")
	}

	return nil
}

// unzip the given go module zip into workDir, skipping any files that aren't
// valid according to modzip.CheckZip or that are potentially malicious.
func unzip(mod module.Version, zipBytes []byte, workDir string) error {
	zipFile := path.Join(workDir, "mod.zip")
	err := os.WriteFile(zipFile, zipBytes, 0o666)
	if err != nil {
		return errors.Wrapf(err, "failed to create go module zip file %q", zipFile)
	}

	files, err := modzip.CheckZip(mod, zipFile)
	if err != nil && len(files.Valid) == 0 {
		return errors.Wrapf(err, "failed to check go module zip %q", zipFile)
	}

	if err = os.RemoveAll(zipFile); err != nil {
		return errors.Wrapf(err, "failed to remove module zip file %q", zipFile)
	}

	if len(files.Valid) == 0 {
		return nil
	}

	valid := make(map[string]struct{}, len(files.Valid))
	for _, f := range files.Valid {
		valid[f] = struct{}{}
	}

	br := bytes.NewReader(zipBytes)
	err = unpack.Zip(br, int64(br.Len()), workDir, unpack.Opts{
		SkipInvalid:    true,
		SkipDuplicates: true,
		Filter: func(path string, file fs.FileInfo) bool {
			malicious := isPotentiallyMaliciousFilepathInArchive(path, workDir)
			_, ok := valid[path]
			return ok && !malicious
		},
	})

	if err != nil {
		return err
	}

	// All files in module zips are prefixed by prefix below, but we don't want
	// those nested directories in our actual repository, so we move all the files up
	// with the below renames.
	tmpDir := workDir + ".tmp"

	// mv $workDir $tmpDir
	err = os.Rename(workDir, tmpDir)
	if err != nil {
		return err
	}

	// mv $tmpDir/$(basename $prefix) $workDir
	prefix := fmt.Sprintf("%s@%s/", mod.Path, mod.Version)
	return os.Rename(path.Join(tmpDir, prefix), workDir)
}
