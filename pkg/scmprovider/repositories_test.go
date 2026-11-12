package scmprovider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/driver/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// githubDefaultPageSize is what GitHub returns when a request sets no per_page.
const githubDefaultPageSize = 30

type githubStatus struct {
	State   string `json:"state"`
	Context string `json:"context"`
}

func TestGetCombinedStatusReturnsEveryContext(t *testing.T) {
	const contextCount = 150

	// Newest first, as GitHub lists them. "job-0" failed, then passed on a rerun.
	latest := []githubStatus{{State: "success", Context: "job-0"}}
	for jobNumber := 1; jobNumber < contextCount; jobNumber++ {
		latest = append(latest, githubStatus{State: "failure", Context: fmt.Sprintf("job-%d", jobNumber)})
	}
	history := append(latest, githubStatus{State: "failure", Context: "job-0"})

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/commits/cafe/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "failure", "sha": "cafe", "statuses": latest[:githubDefaultPageSize]})
	})
	mux.HandleFunc("/repos/org/repo/statuses/cafe", func(w http.ResponseWriter, r *http.Request) {
		pageSize := githubDefaultPageSize
		if requested := r.URL.Query().Get("per_page"); requested != "" {
			pageSize, _ = strconv.Atoi(requested)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		lastPage := (len(history) + pageSize - 1) / pageSize
		if page < lastPage {
			w.Header().Set("Link", fmt.Sprintf(`<%s?page=%d>; rel="last"`, r.URL.Path, lastPage))
		}
		start := min((page-1)*pageSize, len(history))
		end := min(page*pageSize, len(history))
		_ = json.NewEncoder(w).Encode(history[start:end])
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	scmClient, err := github.New(server.URL)
	require.NoError(t, err)

	combined, err := ToTestClient(scmClient).GetCombinedStatus("org", "repo", "cafe")
	require.NoError(t, err)

	require.Len(t, combined.Statuses, contextCount)
	states := map[string]scm.State{}
	for _, status := range combined.Statuses {
		states[status.Label] = status.State
	}
	assert.Equal(t, scm.StateSuccess, states["job-0"], "job-0 must show its newest status")
	assert.Equal(t, scm.StateFailure, states[fmt.Sprintf("job-%d", contextCount-1)])
}
