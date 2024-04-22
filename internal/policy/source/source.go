// Copyright The Enterprise Contract Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// This module is more of a general purpose wrapper for fetching files and
// saving them locally. It was originally used only for policy, i.e. rego and
// yaml files, from the ec-policies repo, hence the name choice for PolicySource,
// but now it's also used for fetching configuration from a git url.

package source

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	ecc "github.com/enterprise-contract/enterprise-contract-controller/api/v1alpha1"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/afero"

	"github.com/enterprise-contract/ec-cli/internal/downloader"
	"github.com/enterprise-contract/ec-cli/internal/utils"
)

type key int
type policyKind string

const (
	DownloaderFuncKey key        = 0
	PolicyKind        policyKind = "policy"
	DataKind          policyKind = "data"
	ConfigKind        policyKind = "config"
)

type downloaderFunc interface {
	Download(context.Context, string, string, bool) error
}

// PolicySource in an interface representing the location a policy source.
// Must implement the GetPolicy() method.
type PolicySource interface {
	GetPolicy(ctx context.Context, dest string, showMsg bool) (string, error)
	PolicyUrl() string
	Subdir() string
}

type PolicyUrl struct {
	// A string containing a go-getter style source url compatible with conftest pull
	Url string
	// Either "data", "policy", or "config"
	Kind policyKind
}

// downloadCache is a concurrent map used to cache downloaded files.
var downloadCache sync.Map

// GetPolicies clones the repository for a given PolicyUrl
func (p *PolicyUrl) GetPolicy(ctx context.Context, workDir string, showMsg bool) (string, error) {
	sourceUrl := p.PolicyUrl()
	dest := uniqueDestination(workDir, p.Subdir(), sourceUrl)

	// Load or store the downloaded policy file from the given source URL.
	// If the file is already in the download cache, it is loaded from there.
	// Otherwise, it is downloaded from the source URL and stored in the cache.
	dfn, _ := downloadCache.LoadOrStore(sourceUrl, sync.OnceValues(func() (string, error) {
		log.Debugf("Download cache miss: %s", sourceUrl)
		// Checkout policy repo into work directory.
		log.Debugf("Downloading policy files from source url %s to destination %s", sourceUrl, dest)
		x := ctx.Value(DownloaderFuncKey)
		if dl, ok := x.(downloaderFunc); ok {
			return dest, dl.Download(ctx, dest, sourceUrl, showMsg)
		}
		return dest, downloader.Download(ctx, dest, sourceUrl, showMsg)
	}))

	d, err := dfn.(func() (string, error))()

	// If the destination directory is different from the source directory, we
	// need to symlink the source directory to the destination directory.
	if filepath.Dir(dest) != filepath.Dir(d) {
		fs := utils.FS(ctx)
		if symlinkableFS, ok := fs.(afero.Symlinker); ok {
			log.Debugf("Symlinking %s to %s", d, dest)
			if err := symlinkableFS.SymlinkIfPossible(d, dest); err == nil {
				return dest, nil
			}
		} else {
			log.Debugf("Filesystem does not support symlinking: %s", fs.Name())
			log.Debugf("The error was: %v", err)
			log.Debugf("Re-downloading instead")
			x := ctx.Value(DownloaderFuncKey)
			if dl, ok := x.(downloaderFunc); ok {
				return dest, dl.Download(ctx, dest, sourceUrl, showMsg)
			}
		}
		return dest, nil
	}
	return d, err
}

func (p *PolicyUrl) PolicyUrl() string {
	return p.Url
}

func (p *PolicyUrl) Subdir() string {
	// Be lazy and assume the kind value is the same as the subdirectory we want
	return string(p.Kind)
}

func uniqueDestination(rootDir string, subdir string, sourceUrl string) string {
	return path.Join(rootDir, subdir, uniqueDir(sourceUrl))
}

// uniqueDir generates a reasonably unique string using an SHA224 sum with a
// timestamp appended to the input for some extra randomness
func uniqueDir(input string) string {
	return fmt.Sprintf("%x", sha256.Sum224([]byte(fmt.Sprintf("%s/%s", input, time.Now()))))[:9]
}

type inlineData struct {
	source []byte
}

func InlineData(source []byte) PolicySource {
	return inlineData{source}
}

func (s inlineData) GetPolicy(ctx context.Context, workDir string, showMsg bool) (string, error) {
	dest := path.Join(workDir, s.Subdir())

	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}

	f := path.Join(dest, "rule_data.json")

	dfn, _ := downloadCache.LoadOrStore(f, sync.OnceValues(func() (string, error) {
		log.Debugf("Inlined rule data cache miss: %s", f)
		return dest, os.WriteFile(f, s.source, 0400)

	}))

	return dfn.(func() (string, error))()
}

func (s inlineData) PolicyUrl() string {
	return "data:application/json;base64," + base64.StdEncoding.EncodeToString(s.source)
}

func (s inlineData) Subdir() string {
	return "data"
}

// FetchPolicySources returns an array of policy sources
func FetchPolicySources(s ecc.Source) ([]PolicySource, error) {
	policySources := make([]PolicySource, 0, len(s.Policy)+len(s.Data))

	for _, policySourceUrl := range s.Policy {
		url := PolicyUrl{Url: policySourceUrl, Kind: "policy"}
		policySources = append(policySources, &url)
	}

	for _, dataSourceUrl := range s.Data {
		url := PolicyUrl{Url: dataSourceUrl, Kind: "data"}
		policySources = append(policySources, &url)
	}

	if s.RuleData != nil {
		data := append(append([]byte(`{"rule_data__configuration__":`), s.RuleData.Raw...), '}')
		policySources = append(policySources, InlineData(data))
	}

	return policySources, nil
}
