//go:build e2e
// +build e2e

/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/test"
	v1test "knative.dev/serving/test/v1"
)

// TestConfigurationGetAndList tests Getting and Listing Configurations resources.
//
//	This test doesn't validate the Data Plane, it is just to check the APIs for conformance
func TestConfigurationGetAndList(t *testing.T) {
	t.Parallel()
	clients := test.Setup(t)

	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   test.PizzaPlanet1,
	}

	// Clean up on test failure or interrupt
	test.EnsureTearDown(t, clients, &names)

	// Setup initial Service
	if _, err := v1test.CreateServiceReady(t, clients, &names); err != nil {
		t.Fatalf("Failed to create initial Service %v: %v", names.Service, err)
	}

	config := fetchConfiguration(names.Config, clients, t)

	list, err := v1test.GetConfigurations(clients)
	if err != nil {
		t.Fatal("Listing Configurations failed")
	}
	if len(list.Items) < 1 {
		t.Fatal("Listing should return at least one Configuration")
	}
	configurationFound := false
	for _, configuration := range list.Items {
		t.Logf("Configuration Returned: %s", configuration.Name)
		if configuration.Name == config.Name {
			configurationFound = true
		}
	}
	if !configurationFound {
		t.Fatal("The Configuration that was previously created was not found by listing all Configurations.")
	}
}

func TestUpdateConfigurationMetadata(t *testing.T) {
	if test.ServingFlags.DisableOptionalAPI {
		t.Skip("Configuration create/patch/replace APIs are not required by Knative Serving API Specification")
	}

	t.Parallel()
	clients := test.Setup(t)

	names := test.ResourceNames{
		Config: test.ObjectNameForTest(t),
		Image:  test.PizzaPlanet1,
	}

	test.EnsureTearDown(t, clients, &names)

	t.Log("Creating new configuration", names.Config)
	if _, err := v1test.CreateConfiguration(t, clients, names); err != nil {
		t.Fatal("Failed to create configuration", names.Config)
	}

	// Wait for the configuration to actually be ready to not race in the updates below.
	if err := v1test.WaitForConfigurationState(clients.ServingClient, names.Config, v1test.IsConfigurationReady, "ConfigurationIsReady"); err != nil {
		t.Fatalf("Configuration %s did not become ready: %v", names.Config, err)
	}

	cfg := fetchConfiguration(names.Config, clients, t)
	names.Revision = cfg.Status.LatestReadyRevisionName

	t.Log("Updating labels of Configuration", names.Config)
	newLabels := map[string]string{
		"label-x": "abc",
		"label-y": "def",
	}
	// Copy over new labels.
	cfg.Labels = kmeta.UnionMaps(cfg.Labels, newLabels)
	cfg, err := clients.ServingClient.Configs.Update(context.Background(), cfg, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update labels for Configuration %s: %v", names.Config, err)
	}

	if err = waitForConfigurationLabelsUpdate(clients, names, cfg.Labels); err != nil {
		t.Fatalf("The labels for Configuration %s were not updated: %v", names.Config, err)
	}

	cfg = fetchConfiguration(names.Config, clients, t)
	expected := names.Revision
	actual := cfg.Status.LatestCreatedRevisionName
	if expected != actual {
		t.Errorf("Did not expect a new Revision after updating labels for Configuration %s - expected Revision: %s, actual Revision: %s",
			names.Config, expected, actual)
	}

	t.Log("Validating labels were not propagated to Revision", names.Revision)
	err = v1test.CheckRevisionState(clients.ServingClient, names.Revision, func(r *v1.Revision) (bool, error) {
		// Labels we placed on Configuration should _not_ appear on Revision.
		return checkNoKeysPresent(newLabels, r.Labels, t), nil
	})
	if err != nil {
		t.Errorf("The labels for Revision %s of Configuration %s should not have been updated: %v", names.Revision, names.Config, err)
	}

	t.Log("Updating annotations of Configuration", names.Config)
	newAnnotations := map[string]string{
		"annotation-a": "123",
		"annotation-b": "456",
	}
	cfg.Annotations = kmeta.UnionMaps(cfg.Annotations, newAnnotations)
	cfg, err = clients.ServingClient.Configs.Update(context.Background(), cfg, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update annotations for Configuration %s: %v", names.Config, err)
	}

	if err = waitForConfigurationAnnotationsUpdate(clients, names, cfg.Annotations); err != nil {
		t.Fatalf("The annotations for Configuration %s were not updated: %v", names.Config, err)
	}

	cfg = fetchConfiguration(names.Config, clients, t)
	actual = cfg.Status.LatestCreatedRevisionName
	if expected != actual {
		t.Errorf("Did not expect a new Revision after updating annotations for Configuration %s - expected Revision: %s, actual Revision: %s",
			names.Config, expected, actual)
	}

	t.Log("Validating annotations were not propagated to Revision", names.Revision)
	err = v1test.CheckRevisionState(clients.ServingClient, names.Revision, func(r *v1.Revision) (bool, error) {
		// Annotations we placed on Configuration should _not_ appear on Revision.
		return checkNoKeysPresent(newAnnotations, r.Annotations, t), nil
	})
	if err != nil {
		t.Errorf("The annotations for Revision %s of Configuration %s should not have been updated: %v", names.Revision, names.Config, err)
	}
}

func fetchConfiguration(name string, clients *test.Clients, t *testing.T) *v1.Configuration {
	cfg, err := clients.ServingClient.Configs.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get configuration %s: %v", name, err)
	}
	return cfg
}

func waitForConfigurationLabelsUpdate(clients *test.Clients, names test.ResourceNames, labels map[string]string) error {
	return v1test.WaitForConfigurationState(clients.ServingClient, names.Config, func(c *v1.Configuration) (bool, error) {
		return cmp.Equal(c.Labels, labels) && c.Generation == c.Status.ObservedGeneration, nil
	}, "ConfigurationMetadataUpdatedWithLabels")
}

func waitForConfigurationAnnotationsUpdate(clients *test.Clients, names test.ResourceNames, annotations map[string]string) error {
	return v1test.WaitForConfigurationState(clients.ServingClient, names.Config, func(c *v1.Configuration) (bool, error) {
		return cmp.Equal(c.Annotations, annotations) && c.Generation == c.Status.ObservedGeneration, nil
	}, "ConfigurationMetadataUpdatedWithAnnotations")
}

// checkNoKeysPresent returns true if _no_ keys from `expected`, are present in `actual`.
// checkNoKeysPresent will log the offending keys to t.Log.
func checkNoKeysPresent(expected, actual map[string]string, t *testing.T) bool {
	t.Helper()
	present := []string{}
	for k := range expected {
		if _, ok := actual[k]; ok {
			present = append(present, k)
		}
	}
	if len(present) != 0 {
		t.Log("Unexpected keys:", present)
	}
	return len(present) == 0
}
