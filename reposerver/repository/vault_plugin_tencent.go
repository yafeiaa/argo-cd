package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/argoproj/argo-cd/v3/common"
	"github.com/ulule/deepcopier"

	"github.com/argoproj/argo-cd/v3/reposerver/apiclient"
	log "github.com/sirupsen/logrus"
)

type VaultManifestReplaceResponse struct {
	Manifests []string `json:"manifests"`
	Message   string   `json:"message"`
}

func afterGenerateManifestHookServer() string {
	return os.Getenv(common.EnvAfterGenerateMfstHookServer)
}

func afterGenerateManifest(mfreq *apiclient.ManifestRequest, mfst *apiclient.ManifestResponse) (*apiclient.ManifestResponse, error) {
	newmfst := &apiclient.ManifestResponse{}
	deepcopier.Copy(mfst).To(newmfst)

	repos := []string{}
	if mfreq.HasMultipleSources {
		for _, r := range mfreq.Repos {
			repos = append(repos, r.Repo)
		}
	} else {
		repos = append(repos, mfreq.Repo.Repo)
	}

	data := map[string]interface{}{
		"manifests": mfst.Manifests,
		"project":   mfreq.ProjectName,
		"appName":   mfreq.AppName,
		"repos":     repos,
	}
	if mfst.Manifests == nil {
		log.Warnf("because manifests is null, so skip to call vault-plugin, params:%v", data)
		return mfst, nil
	}
	body, err := json.Marshal(data)
	if err != nil {
		return newmfst, fmt.Errorf("marshal data failed: %w", err)
	}
	addr := afterGenerateManifestHookServer()
	if addr == "" {
		return newmfst, fmt.Errorf("afterGenerateManifestHookServer address is empty")
	}
	req, err := http.NewRequest(http.MethodPost, addr, bytes.NewReader(body))
	if err != nil {
		return newmfst, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return newmfst, fmt.Errorf("do request failed: %w", err)
	}
	defer resp.Body.Close()
	respbody, err := io.ReadAll(resp.Body)
	if err != nil {
		return mfst, fmt.Errorf("read response body failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return mfst, fmt.Errorf("call generatemanifesthook failed with response code '%d' when request vault plugin: %s", resp.StatusCode, string(respbody))
	}
	response := VaultManifestReplaceResponse{}
	err = json.Unmarshal(respbody, &response)
	if err != nil {
		return mfst, fmt.Errorf("unmarshal body failed, body is: %s: %w", string(respbody), err)
	}
	newmfst.Manifests = response.Manifests
	return newmfst, nil
}
