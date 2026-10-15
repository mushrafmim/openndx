package federator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDevelopmentConfigArgumentMappings exercises the actual local configs and
// schema so the walkthrough's NIC reaches each selected provider.
func TestDevelopmentConfigArgumentMappings(t *testing.T) {
	schema, err := os.ReadFile("../../../cmd/oe/schema.graphql")
	require.NoError(t, err)
	sdl := string(schema)
	for _, configFile := range []string{"config.docker.json", "config.example.json"} {
		for _, tc := range []struct {
			name      string
			request   graphql.Request
			providers []string
			want      map[string]interface{}
		}{
			{
				name:      "public DRP literal",
				request:   graphql.Request{Query: `query { personInfo(nic: "748213") { fullName } }`},
				providers: []string{"drp"},
				want:      map[string]interface{}{"fullName": "Suresh"},
			},
			{
				name: "public DRP variable",
				request: graphql.Request{
					Query:     `query Person($nic: String!) { personInfo(nic: $nic) { fullName } }`,
					Variables: map[string]interface{}{"nic": "748213"},
				},
				providers: []string{"drp"},
				want:      map[string]interface{}{"fullName": "Suresh"},
			},
			{
				name: "RGD variable",
				request: graphql.Request{
					Query:     `query Person($nic: String!) { personInfo(nic: $nic) { birthInfo { district } } }`,
					Variables: map[string]interface{}{"nic": "748213"},
				},
				providers: []string{"rgd"},
				want:      map[string]interface{}{"birthInfo": map[string]interface{}{"district": "Rivertown"}},
			},
			{
				name:      "both providers",
				request:   graphql.Request{Query: `query { personInfo(nic: "748213") { fullName profession birthInfo { birthPlace district } } }`},
				providers: []string{"drp", "rgd"},
				want: map[string]interface{}{
					"fullName": "Suresh", "profession": "Software Engineer",
					"birthInfo": map[string]interface{}{"birthPlace": "Rivertown General Hospital", "district": "Rivertown"},
				},
			},
		} {
			t.Run(configFile+"/"+tc.name, func(t *testing.T) {
				cfg, err := configs.LoadConfigFile("../../../cmd/oe/" + configFile)
				require.NoError(t, err)
				cfg.Schema = &sdl
				pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(policy.PdpResponse{AppAuthorized: true})
				}))
				t.Cleanup(pdp.Close)
				cfg.PdpConfig.ClientURL = pdp.URL
				calls := make(chan string, 2)
				for _, p := range cfg.Providers {
					key := p.ProviderKey
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var request graphql.Request
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Errorf("decode %s request: %v", key, err)
							http.Error(w, "invalid request", http.StatusBadRequest)
							return
						}
						if !assert.Contains(t, request.Query, `nic: "748213"`) {
							http.Error(w, "missing NIC", http.StatusBadRequest)
							return
						}
						calls <- key
						var data map[string]interface{}
						if key == "drp" {
							data = map[string]interface{}{"person": map[string]interface{}{"fullName": "Suresh", "profession": "Software Engineer"}}
						} else {
							data = map[string]interface{}{"getPersonInfo": map[string]interface{}{"birthPlace": "Rivertown General Hospital", "district": "Rivertown"}}
						}
						_ = json.NewEncoder(w).Encode(graphql.Response{Data: data})
					}))
					t.Cleanup(server.Close)
					p.ProviderURL = server.URL
					p.Auth = nil
				}
				f, err := Initialize(context.Background(), cfg, provider.NewProviderHandler(nil), nil)
				require.NoError(t, err)
				response := f.FederateQuery(context.Background(), tc.request, &auth.ConsumerAssertion{ApplicationID: "demo-client"})
				require.Empty(t, response.Errors)
				require.Equal(t, tc.want, response.Data["personInfo"])
				require.Len(t, calls, len(tc.providers))
				got := make([]string, 0, len(tc.providers))
				for range tc.providers {
					got = append(got, <-calls)
				}
				require.ElementsMatch(t, tc.providers, got)
			})
		}
	}
}
