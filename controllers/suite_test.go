/*
Copyright 2022.

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

package controllers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kube-champ/terraform-operator/api/v1alpha1"
	"github.com/kube-champ/terraform-operator/pkg/kube"
	"github.com/kube-champ/terraform-operator/pkg/utils"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	os.Setenv("DOCKER_REGISTRY", "docker.io")
	os.Setenv("TERRAFORM_RUNNER_IMAGE", "ibraheemalsaady/terraform-runner")
	os.Setenv("TERRAFORM_RUNNER_IMAGE_TAG", "0.0.3")
	os.Setenv("KNOWN_HOSTS_CONFIGMAP_NAME", "operator-known-hosts")

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})

	Expect(err).ToNot(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	err = (&TerraformReconciler{
		Client:   k8sClient,
		Recorder: k8sManager.GetEventRecorderFor("terraform-controller"),
		Log:      ctrl.Log.WithName("controllers").WithName("TerraformController"),
	}).SetupWithManager(k8sManager)

	kube.ClientSet = fake.NewSimpleClientset()
	utils.LoadEnv()

	Expect(err).NotTo(HaveOccurred(), "failed to setup controller in test")

	err = prepareRunnerRBAC()
	Expect(err).ToNot(HaveOccurred(), "could not prepare rbac for terraform runner")

	go func() {
		defer GinkgoRecover()

		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred(), "failed to start manager")

		gexec.KillAndWait(4 * time.Second)

		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	}()

}, 60)

// var _ = AfterSuite(func() {
// 	By("tearing down the test environment")
// 	err := testEnv.Stop()
// 	Expect(err).NotTo(HaveOccurred())
// })
