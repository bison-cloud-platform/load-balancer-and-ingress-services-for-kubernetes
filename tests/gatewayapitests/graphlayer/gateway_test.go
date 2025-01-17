/*
 * Copyright 2023-2024 VMware, Inc.
 * All Rights Reserved.
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*   http://www.apache.org/licenses/LICENSE-2.0
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*/

package graphlayer

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	akogatewayapik8s "github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/ako-gateway-api/k8s"
	akogatewayapilib "github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/ako-gateway-api/lib"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/internal/k8s"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/internal/lib"
	avinodes "github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/internal/nodes"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/internal/objects"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/pkg/utils"
	tests "github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/tests/gatewayapitests"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/tests/integrationtest"
)

var ctrl *akogatewayapik8s.GatewayController

const (
	DEFAULT_NAMESPACE = "default"
)

func TestMain(m *testing.M) {
	tests.KubeClient = k8sfake.NewSimpleClientset()
	tests.GatewayClient = gatewayfake.NewSimpleClientset()
	integrationtest.KubeClient = tests.KubeClient

	// Sets the environment variables
	os.Setenv("CLUSTER_NAME", "cluster")
	os.Setenv("CLOUD_NAME", "CLOUD_VCENTER")
	os.Setenv("SEG_NAME", "Default-Group")
	os.Setenv("POD_NAMESPACE", utils.AKO_DEFAULT_NS)
	os.Setenv("FULL_SYNC_INTERVAL", utils.AKO_DEFAULT_NS)
	os.Setenv("ENABLE_EVH", "true")
	os.Setenv("TENANT", "admin")

	// Set the user with prefix
	_ = lib.AKOControlConfig()
	lib.SetAKOUser(akogatewayapilib.Prefix)
	lib.SetNamePrefix(akogatewayapilib.Prefix)
	lib.AKOControlConfig().SetIsLeaderFlag(true)
	akoControlConfig := akogatewayapilib.AKOControlConfig()
	akoControlConfig.SetEventRecorder(lib.AKOGatewayEventComponent, tests.KubeClient, true)
	registeredInformers := []string{
		utils.ServiceInformer,
		utils.EndpointInformer,
		utils.SecretInformer,
		utils.NSInformer,
	}
	utils.AviLog.SetLevel("DEBUG")
	utils.NewInformers(utils.KubeClientIntf{ClientSet: tests.KubeClient}, registeredInformers, make(map[string]interface{}))
	data := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("admin"),
	}
	object := metav1.ObjectMeta{Name: "avi-secret", Namespace: utils.GetAKONamespace()}
	secret := &corev1.Secret{Data: data, ObjectMeta: object}
	tests.KubeClient.CoreV1().Secrets(utils.GetAKONamespace()).Create(context.TODO(), secret, metav1.CreateOptions{})

	akoApi := integrationtest.InitializeFakeAKOAPIServer()
	defer akoApi.ShutDown()

	tests.NewAviFakeClientInstance(tests.KubeClient)
	defer integrationtest.AviFakeClientInstance.Close()

	ctrl = akogatewayapik8s.SharedGatewayController()
	ctrl.DisableSync = false
	ctrl.InitGatewayAPIInformers(tests.GatewayClient)
	akoControlConfig.SetGatewayAPIClientset(tests.GatewayClient)

	stopCh := utils.SetupSignalHandler()
	ctrlCh := make(chan struct{})
	quickSyncCh := make(chan struct{})

	waitGroupMap := make(map[string]*sync.WaitGroup)
	wgIngestion := &sync.WaitGroup{}
	waitGroupMap["ingestion"] = wgIngestion
	wgFastRetry := &sync.WaitGroup{}
	waitGroupMap["fastretry"] = wgFastRetry
	wgSlowRetry := &sync.WaitGroup{}
	waitGroupMap["slowretry"] = wgSlowRetry
	wgGraph := &sync.WaitGroup{}
	waitGroupMap["graph"] = wgGraph
	wgStatus := &sync.WaitGroup{}
	waitGroupMap["status"] = wgStatus

	integrationtest.AddConfigMap(tests.KubeClient)
	go ctrl.InitController(k8s.K8sinformers{Cs: tests.KubeClient}, registeredInformers, ctrlCh, stopCh, quickSyncCh, waitGroupMap)
	os.Exit(m.Run())
}

/*
Positive Case
Create Gateway 1 listener (noTLS)
Create Gateway 1 listener (TLS)
*/
func TestGateway(t *testing.T) {

	gatewayName := "gateway-01"
	gatewayClassName := "gateway-class-01"
	ports := []int32{8080}

	tests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	listeners := tests.GetListenersV1Beta1(ports)
	tests.SetupGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g := gomega.NewGomegaWithT(t)

	g.Eventually(func() bool {
		gateway, err := tests.GatewayClient.GatewayV1beta1().Gateways(DEFAULT_NAMESPACE).Get(context.TODO(), gatewayName, metav1.GetOptions{})
		if err != nil || gateway == nil {
			t.Logf("Couldn't get the gateway, err: %+v", err)
			return false
		}
		return apimeta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1beta1.GatewayConditionAccepted)) != nil
	}, 30*time.Second).Should(gomega.Equal(true))

	modelName := lib.GetModelName(lib.GetTenant(), akogatewayapilib.GetGatewayParentName(DEFAULT_NAMESPACE, gatewayName))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 25*time.Second).Should(gomega.Equal(true))

	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
	g.Expect(nodes).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto).To(gomega.HaveLen(1))
	g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(0))
	g.Expect(nodes[0].VSVIPRefs).To(gomega.HaveLen(1))

	tests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)
	tests.TeardownGatewayClass(t, gatewayClassName)
}

func TestGatewayWithTLS(t *testing.T) {

	gatewayName := "gateway-02"
	gatewayClassName := "gateway-class-02"
	ports := []int32{8080}

	secrets := []string{"secret-01"}
	for _, secret := range secrets {
		integrationtest.AddSecret(secret, DEFAULT_NAMESPACE, "cert", "key")
	}

	tests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	listeners := tests.GetListenersV1Beta1(ports, secrets...)
	tests.SetupGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g := gomega.NewGomegaWithT(t)

	g.Eventually(func() bool {
		gateway, err := tests.GatewayClient.GatewayV1beta1().Gateways(DEFAULT_NAMESPACE).Get(context.TODO(), gatewayName, metav1.GetOptions{})
		if err != nil || gateway == nil {
			t.Logf("Couldn't get the gateway, err: %+v", err)
			return false
		}
		return apimeta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1beta1.GatewayConditionAccepted)) != nil
	}, 30*time.Second).Should(gomega.Equal(true))

	modelName := lib.GetModelName(lib.GetTenant(), akogatewayapilib.GetGatewayParentName(DEFAULT_NAMESPACE, gatewayName))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 25*time.Second).Should(gomega.Equal(true))

	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
	g.Expect(nodes).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto[0].EnableSSL).To(gomega.Equal(true))
	g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(1))
	g.Expect(nodes[0].VSVIPRefs).To(gomega.HaveLen(1))

	tests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)
	tests.TeardownGatewayClass(t, gatewayClassName)
	integrationtest.DeleteSecret(secrets[0], DEFAULT_NAMESPACE)
}

/*
Positive Case
Transition Gateway 1 listener (noTLS -> TLS)
Transition Gateway 1 listener (TLS -> noTLS)
*/

func TestGatewayNoTLSToTLS(t *testing.T) {

	gatewayName := "gateway-03"
	gatewayClassName := "gateway-class-03"
	ports := []int32{8080}

	tests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	listeners := tests.GetListenersV1Beta1(ports)
	tests.SetupGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g := gomega.NewGomegaWithT(t)

	g.Eventually(func() bool {
		gateway, err := tests.GatewayClient.GatewayV1beta1().Gateways(DEFAULT_NAMESPACE).Get(context.TODO(), gatewayName, metav1.GetOptions{})
		if err != nil || gateway == nil {
			t.Logf("Couldn't get the gateway, err: %+v", err)
			return false
		}
		return apimeta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1beta1.GatewayConditionAccepted)) != nil
	}, 30*time.Second).Should(gomega.Equal(true))

	modelName := lib.GetModelName(lib.GetTenant(), akogatewayapilib.GetGatewayParentName(DEFAULT_NAMESPACE, gatewayName))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 25*time.Second).Should(gomega.Equal(true))

	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
	g.Expect(nodes).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto).To(gomega.HaveLen(1))
	g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(0))
	g.Expect(nodes[0].VSVIPRefs).To(gomega.HaveLen(1))

	//var tlsMode gatewayv1beta1.TLSModeType
	tlsMode := gatewayv1beta1.TLSModeTerminate
	secrets := []string{"secret-02"}
	for _, secret := range secrets {
		integrationtest.AddSecret(secret, DEFAULT_NAMESPACE, "cert", "key")
	}
	certRefs := []gatewayv1beta1.SecretObjectReference{{Name: gatewayv1beta1.ObjectName(secrets[0])}}
	listeners[0].TLS = &gatewayv1beta1.GatewayTLSConfig{
		Mode:            &tlsMode,
		CertificateRefs: certRefs,
	}
	tests.UpdateGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g.Eventually(func() bool {
		gateway, err := tests.GatewayClient.GatewayV1beta1().Gateways(DEFAULT_NAMESPACE).Get(context.TODO(), gatewayName, metav1.GetOptions{})
		if err != nil || gateway == nil {
			t.Logf("Couldn't get the gateway, err: %+v", err)
			return false
		}
		return apimeta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1beta1.GatewayConditionAccepted)) != nil
	}, 30*time.Second).Should(gomega.Equal(true))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 25*time.Second).Should(gomega.Equal(true))

	_, aviModel = objects.SharedAviGraphLister().Get(modelName)
	nodes = aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
	g.Expect(nodes).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto[0].EnableSSL).To(gomega.Equal(true))
	g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(1))

	tests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)
	tests.TeardownGatewayClass(t, gatewayClassName)
	integrationtest.DeleteSecret(secrets[0], DEFAULT_NAMESPACE)
}

func TestGatewayTLSToNoTLS(t *testing.T) {

	gatewayName := "gateway-04"
	gatewayClassName := "gateway-class-04"
	ports := []int32{8080}

	secrets := []string{"secret-03"}
	for _, secret := range secrets {
		integrationtest.AddSecret(secret, DEFAULT_NAMESPACE, "cert", "key")
	}

	tests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	listeners := tests.GetListenersV1Beta1(ports, secrets...)
	tests.SetupGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g := gomega.NewGomegaWithT(t)

	g.Eventually(func() bool {
		gateway, err := tests.GatewayClient.GatewayV1beta1().Gateways(DEFAULT_NAMESPACE).Get(context.TODO(), gatewayName, metav1.GetOptions{})
		if err != nil || gateway == nil {
			t.Logf("Couldn't get the gateway, err: %+v", err)
			return false
		}
		return apimeta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1beta1.GatewayConditionAccepted)) != nil
	}, 30*time.Second).Should(gomega.Equal(true))

	modelName := lib.GetModelName(lib.GetTenant(), akogatewayapilib.GetGatewayParentName(DEFAULT_NAMESPACE, gatewayName))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 25*time.Second).Should(gomega.Equal(true))

	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
	g.Expect(nodes).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto[0].EnableSSL).To(gomega.Equal(true))
	g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(1))
	g.Expect(nodes[0].VSVIPRefs).To(gomega.HaveLen(1))

	listeners[0].TLS = nil
	tests.UpdateGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g.Eventually(func() bool {
		gateway, err := tests.GatewayClient.GatewayV1beta1().Gateways(DEFAULT_NAMESPACE).Get(context.TODO(), gatewayName, metav1.GetOptions{})
		if err != nil || gateway == nil {
			t.Logf("Couldn't get the gateway, err: %+v", err)
			return false
		}
		return apimeta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1beta1.GatewayConditionAccepted)) != nil
	}, 30*time.Second).Should(gomega.Equal(true))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 25*time.Second).Should(gomega.Equal(true))

	_, aviModel = objects.SharedAviGraphLister().Get(modelName)
	nodes = aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
	g.Expect(nodes).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto).To(gomega.HaveLen(1))
	g.Expect(nodes[0].PortProto[0].EnableSSL).To(gomega.Equal(false))
	g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(0))

	tests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)
	tests.TeardownGatewayClass(t, gatewayClassName)
	integrationtest.DeleteSecret(secrets[0], DEFAULT_NAMESPACE)
}

/*
Negative Case
Delete Gateway 1 listener
*/

func TestGatewayDelete(t *testing.T) {

	gatewayName := "gateway-05"
	gatewayClassName := "gateway-class-05"
	ports := []int32{8080}

	tests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	listeners := tests.GetListenersV1Beta1(ports)
	tests.SetupGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g := gomega.NewGomegaWithT(t)

	g.Eventually(func() bool {
		gateway, err := tests.GatewayClient.GatewayV1beta1().Gateways(DEFAULT_NAMESPACE).Get(context.TODO(), gatewayName, metav1.GetOptions{})
		if err != nil || gateway == nil {
			t.Logf("Couldn't get the gateway, err: %+v", err)
			return false
		}
		return apimeta.FindStatusCondition(gateway.Status.Conditions, string(gatewayv1beta1.GatewayConditionAccepted)) != nil
	}, 30*time.Second).Should(gomega.Equal(true))

	modelName := lib.GetModelName(lib.GetTenant(), akogatewayapilib.GetGatewayParentName(DEFAULT_NAMESPACE, gatewayName))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 25*time.Second).Should(gomega.Equal(true))

	tests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)

	g.Eventually(func() bool {
		found, gwModel := objects.SharedAviGraphLister().Get(modelName)
		if found {
			return gwModel == nil
		}
		return true
	}, 25*time.Second).Should(gomega.Equal(true))

	tests.TeardownGatewayClass(t, gatewayClassName)
}

func TestSecretCreateDelete(t *testing.T) {

	gatewayName := "gateway-06"
	gatewayClassName := "gateway-class-06"
	ports := []int32{8080}
	secrets := []string{"secret-06"}

	tests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	listeners := tests.GetListenersV1Beta1(ports, secrets...)
	tests.SetupGateway(t, gatewayName, DEFAULT_NAMESPACE, gatewayClassName, nil, listeners)

	g := gomega.NewGomegaWithT(t)
	modelName := lib.GetModelName(lib.GetTenant(), akogatewayapilib.GetGatewayParentName(DEFAULT_NAMESPACE, gatewayName))

	g.Eventually(func() bool {
		found, _ := objects.SharedAviGraphLister().Get(modelName)
		return found
	}, 30*time.Second).Should(gomega.Equal(true))

	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
	g.Expect(nodes).To(gomega.HaveLen(1))
	g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(0))

	integrationtest.AddSecret(secrets[0], DEFAULT_NAMESPACE, "cert", "key")

	g.Eventually(func() bool {
		found, aviModel := objects.SharedAviGraphLister().Get(modelName)
		if found {
			nodes = aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
			g.Expect(nodes).To(gomega.HaveLen(1))
			g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(1))
			return true
		}
		return found
	}, 30*time.Second).Should(gomega.Equal(true))

	// delete
	integrationtest.DeleteSecret(secrets[0], DEFAULT_NAMESPACE)

	g.Eventually(func() bool {
		found, aviModel := objects.SharedAviGraphLister().Get(modelName)
		if found {
			nodes = aviModel.(*avinodes.AviObjectGraph).GetAviEvhVS()
			g.Expect(nodes).To(gomega.HaveLen(1))
			g.Expect(nodes[0].SSLKeyCertRefs).To(gomega.HaveLen(0))
			return true
		}
		return found
	}, 30*time.Second).Should(gomega.Equal(true))

	tests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)
	tests.TeardownGatewayClass(t, gatewayClassName)
}
