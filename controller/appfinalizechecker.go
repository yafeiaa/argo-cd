/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"

	log "github.com/sirupsen/logrus"

	pkgapiclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	applicationsetpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/applicationset"
	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

type AppFinalizeChecker interface {
	CheckAppRefreshedByAppSet(logCtx *log.Entry, ctx context.Context, app *appv1alpha1.Application) error
}

func NewAppFinalizeChecker(argoServerClient pkgapiclient.Client) AppFinalizeChecker {
	re, _ := regexp.Compile(`^{{ .* }}$`)
	return &appChecker{
		revisionRegexp:   re,
		argoServerClient: argoServerClient,
	}
}

type appChecker struct {
	revisionRegexp   *regexp.Regexp
	argoServerClient pkgapiclient.Client
}

func (c *appChecker) CheckAppRefreshedByAppSet(logCtx *log.Entry, ctx context.Context, app *appv1alpha1.Application) error {
	if app == nil || app.Spec.SyncPolicy == nil || !app.Spec.SyncPolicy.SyncOptions.HasOption("WaitAppSetRefresh=true") {
		return nil
	}
	var belongAppSet bool
	var appSetName string
	for i := range app.ObjectMeta.OwnerReferences {
		owner := app.ObjectMeta.OwnerReferences[i]
		if owner.Kind == "ApplicationSet" {
			belongAppSet = true
			appSetName = owner.Name
			break
		}
	}
	if !belongAppSet || appSetName == "" {
		return nil
	}

	// get the appset gRPC client
	closer, appSetClient, err := c.argoServerClient.NewApplicationSetClient()
	if err != nil {
		return fmt.Errorf("failed to create appset client: %w", err)
	}
	defer closer.Close()
	appSet, err := appSetClient.Get(ctx, &applicationsetpkg.ApplicationSetGetQuery{
		Name:            appSetName,
		AppsetNamespace: app.Namespace,
	})
	if err != nil {
		return fmt.Errorf("error getting appset %q: %w", appSetName, err)
	}
	if !c.checkAppSetHasDynamicRevision(appSet) {
		return nil
	}

	// generate appset, compare application with genereated-application
	logCtx.Infof("Starting generate appset '%s' and compare with app '%s'", appSetName, app.Name)
	resp, err := appSetClient.Generate(ctx, &applicationsetpkg.ApplicationSetGenerateRequest{
		ApplicationSet: appSet,
	})
	if err != nil {
		return fmt.Errorf("error generating appset %q: %w", appSetName, err)
	}
	var foundApp *appv1alpha1.Application
	for _, generatedApp := range resp.Applications {
		if generatedApp.Name != app.Name {
			continue
		}
		foundApp = generatedApp
		break
	}
	if foundApp == nil {
		return nil
	}
	if reflect.DeepEqual(foundApp.Spec, app.Spec) {
		return nil
	}
	foundAppBS, err := json.Marshal(foundApp.Spec)
	if err != nil {
		return fmt.Errorf("error marshaling found_application %q: %w", foundApp.Name, err)
	}
	appBS, err := json.Marshal(app.Spec)
	if err != nil {
		return fmt.Errorf("error marshaling target_application %q: %w", app.Name, err)
	}
	return fmt.Errorf("application %q not update to latest, app.spec is inconsistent: original(%s), generated(%s)",
		app.Name, string(appBS), string(foundAppBS))
}

func (c *appChecker) checkAppSetHasDynamicRevision(appSet *appv1alpha1.ApplicationSet) bool {
	appSetTemplate := appSet.Spec.Template
	if appSetTemplate.Spec.HasMultipleSources() {
		for i := range appSetTemplate.Spec.Sources {
			if c.revisionRegexp.MatchString(appSetTemplate.Spec.Sources[i].TargetRevision) {
				return true
			}
		}
	} else if c.revisionRegexp.MatchString(appSetTemplate.Spec.Source.TargetRevision) {
		return true
	}
	return false
}
