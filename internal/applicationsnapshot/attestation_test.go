// Copyright 2023 Red Hat, Inc.
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

package applicationsnapshot

import (
	"mime/multipart"
	"strings"
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	app "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/enterprise-contract/ec-cli/internal/attestation"
	"github.com/enterprise-contract/ec-cli/internal/signature"
)

func TestAttestationReport(t *testing.T) {
	adapt = func(w *multipart.Writer) {
		require.NoError(t, w.SetBoundary("test-boundary"))
	}

	cases := []struct {
		name       string
		components []Component
	}{
		{
			name:       "no components",
			components: []Component{},
		},
		{
			name: "no attestations",
			components: []Component{
				{},
				{},
			},
		},
		{
			name: "one attestation",
			components: []Component{
				{
					SnapshotComponent: app.SnapshotComponent{
						ContainerImage: "registry.io/repository/image:tag",
					},
					Attestations: []attestation.Attestation{
						att("attestation1"),
					},
				},
			},
		},
		{
			name: "two components two attestations",
			components: []Component{
				{
					SnapshotComponent: app.SnapshotComponent{
						ContainerImage: "registry.io/repository/image1:tag",
					},
					Attestations: []attestation.Attestation{
						att("attestation1"),
						att("attestation2"),
					},
				},
				{
					SnapshotComponent: app.SnapshotComponent{
						ContainerImage: "registry.io/repository/image2:tag",
					},
					Attestations: []attestation.Attestation{
						att("attestation3"),
						att("attestation4"),
					},
				},
			},
		},
		{
			name: "mix of components and attestations",
			components: []Component{
				{
					SnapshotComponent: app.SnapshotComponent{
						ContainerImage: "registry.io/repository/image1:tag",
					},
					Attestations: []attestation.Attestation{
						att("attestation1"),
					},
				},
				{
					SnapshotComponent: app.SnapshotComponent{
						ContainerImage: "registry.io/repository/image2:tag",
					},
				},
				{
					SnapshotComponent: app.SnapshotComponent{
						ContainerImage: "registry.io/repository/image3:tag",
					},
					Attestations: []attestation.Attestation{
						att("attestation2"),
						att("attestation3"),
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Report{
				Components: c.components,
			}
			b, err := r.renderAttestations()
			assert.NoError(t, err)

			snaps.MatchSnapshot(t, strings.ReplaceAll(string(b), "\r", "\\r"))
		})
	}
}

type mockAttestation struct {
	data string
}

func (a mockAttestation) Type() string {
	return "type"
}

func (a mockAttestation) PredicateType() string {
	return "predicateType"
}

func (a mockAttestation) Statement() []byte {
	return []byte(a.data)
}

func (a mockAttestation) Signatures() []signature.EntitySignature {
	return nil
}

func (a mockAttestation) ContentType() string {
	return "text/plain"
}

func att(data string) attestation.Attestation {
	return &mockAttestation{
		data: data,
	}
}
