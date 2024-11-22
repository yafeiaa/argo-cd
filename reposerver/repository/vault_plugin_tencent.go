package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/argoproj/argo-cd/v2/common"
	"github.com/ulule/deepcopier"

	"github.com/argoproj/argo-cd/v2/reposerver/apiclient"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ValutManifestReplaceResponse struct {
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
		log.Warnf("because  manifests is null, so skip to call vault-plugin,params:%v", data)
		return mfst, nil
	}
	body, err := json.Marshal(data)
	if err != nil {
		return newmfst, errors.Wrapf(err, " marshal data failed")
	}
	addr := afterGenerateManifestHookServer()
	if addr == "" {
		return newmfst, fmt.Errorf("afterGenerateManifestHookServer address is empty")
	}
	req, err := http.NewRequest(http.MethodPost, addr, bytes.NewReader(body))
	if err != nil {
		return newmfst, errors.Wrapf(err, "create request failed")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return newmfst, errors.Wrapf(err, "do request failed")
	}
	defer resp.Body.Close()
	respbody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return mfst, errors.Wrapf(err, "read response body failed")
	}
	if resp.StatusCode != http.StatusOK {
		return mfst, fmt.Errorf("call generatemanifesthook failed with response code '%d' when request vault plugin: %s", resp.StatusCode, string(respbody))
	}
	response := ValutManifestReplaceResponse{}
	err = json.Unmarshal(respbody, &response)
	if err != nil {
		return mfst, errors.Wrapf(err, "unmarshal body failed, body is: %s", string(respbody))
	}
	newmfst.Manifests = response.Manifests
	return newmfst, nil
}
