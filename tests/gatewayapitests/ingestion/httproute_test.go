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

package ingestion

import (
	"testing"

	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	akogatewayapilib "github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/ako-gateway-api/lib"
	akogatewayapiobjects "github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/ako-gateway-api/objects"
	akogatewayapitests "github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/tests/gatewayapitests"
)

func TestHTTPRouteCUD(t *testing.T) {
	gatewayClassName := "gateway-class-01"
	gatewayName := "gateway-01"
	httpRouteName := "httproute-01"
	namespace := "default"
	ports := []int32{8080, 8081}
	key := "HTTPRoute" + "/" + namespace + "/" + httpRouteName
	akogatewayapiobjects.GatewayApiLister().UpdateGatewayClass(gatewayClassName, true)

	akogatewayapitests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	t.Logf("Created GatewayClass %s", gatewayClassName)
	waitAndverify(t, "GatewayClass/gateway-class-01")

	listeners := akogatewayapitests.GetListenersV1Beta1(ports)
	akogatewayapitests.SetupGateway(t, gatewayName, namespace, gatewayClassName, nil, listeners)
	t.Logf("Created Gateway %s", gatewayName)
	waitAndverify(t, "Gateway/default/gateway-01")

	parentRefs := akogatewayapitests.GetParentReferencesV1Beta1([]string{gatewayName}, namespace, ports)
	hostnames := []gatewayv1beta1.Hostname{"foo-8080.com", "foo-8081.com"}
	akogatewayapitests.SetupHTTPRoute(t, httpRouteName, namespace, parentRefs, hostnames, nil)
	waitAndverify(t, key)

	// update
	hostnames = []gatewayv1beta1.Hostname{"foo-8080.com"}
	akogatewayapitests.UpdateHTTPRoute(t, httpRouteName, namespace, parentRefs, hostnames, nil)
	waitAndverify(t, key)

	// delete
	akogatewayapitests.TeardownHTTPRoute(t, httpRouteName, namespace)
	waitAndverify(t, key)
}

func TestHTTPRouteInvalidHostname(t *testing.T) {
	gatewayClassName := "gateway-class-02"
	gatewayName := "gateway-02"
	httpRouteName := "httproute-02"
	gwKey := "Gateway/" + DEFAULT_NAMESPACE + "/" + gatewayName
	gwClassKey := "GatewayClass/" + gatewayClassName
	namespace := "default"
	ports := []int32{8080}
	key := "HTTPRoute" + "/" + namespace + "/" + httpRouteName
	akogatewayapiobjects.GatewayApiLister().UpdateGatewayClass(gatewayClassName, true)

	akogatewayapitests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	t.Logf("Created GatewayClass %s", gatewayClassName)
	waitAndverify(t, gwClassKey)

	listeners := akogatewayapitests.GetListenersV1Beta1(ports)
	akogatewayapitests.SetupGateway(t, gatewayName, namespace, gatewayClassName, nil, listeners)
	t.Logf("Created Gateway %s", gatewayName)
	waitAndverify(t, gwKey)

	parentRefs := akogatewayapitests.GetParentReferencesV1Beta1([]string{gatewayName}, namespace, ports)
	hostnames := []gatewayv1beta1.Hostname{"*.example.com"}
	akogatewayapitests.SetupHTTPRoute(t, httpRouteName, namespace, parentRefs, hostnames, nil)
	waitAndverify(t, "")

	// update
	hostnames = []gatewayv1beta1.Hostname{"foo-8080.com"}
	akogatewayapitests.UpdateHTTPRoute(t, httpRouteName, namespace, parentRefs, hostnames, nil)
	waitAndverify(t, key)

	// delete
	akogatewayapitests.TeardownHTTPRoute(t, httpRouteName, namespace)
	waitAndverify(t, key)
	akogatewayapitests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)
	waitAndverify(t, gwKey)
	akogatewayapitests.TeardownGatewayClass(t, gatewayClassName)
	waitAndverify(t, gwClassKey)
}

func TestHTTPRouteGatewayNotPresent(t *testing.T) {
	gatewayClassName := "gateway-class-03"
	gatewayName := "gateway-03"
	httpRouteName := "httproute-03"
	gwKey := "Gateway/" + DEFAULT_NAMESPACE + "/" + gatewayName
	gwClassKey := "GatewayClass/" + gatewayClassName
	namespace := "default"
	ports := []int32{8080, 8081}
	key := "HTTPRoute" + "/" + namespace + "/" + httpRouteName
	akogatewayapiobjects.GatewayApiLister().UpdateGatewayClass(gatewayClassName, true)

	akogatewayapitests.SetupGatewayClass(t, gatewayClassName, akogatewayapilib.GatewayController)
	t.Logf("Created GatewayClass %s", gatewayClassName)
	waitAndverify(t, gwClassKey)

	parentRefs := akogatewayapitests.GetParentReferencesV1Beta1([]string{gatewayName}, namespace, ports)
	hostnames := []gatewayv1beta1.Hostname{"foo-8080.com", "foo-8081.com"}
	akogatewayapitests.SetupHTTPRoute(t, httpRouteName, namespace, parentRefs, hostnames, nil)
	waitAndverify(t, "")

	// update
	listeners := akogatewayapitests.GetListenersV1Beta1(ports)
	akogatewayapitests.SetupGateway(t, gatewayName, namespace, gatewayClassName, nil, listeners)
	t.Logf("Created Gateway %s", gatewayName)
	waitAndverify(t, gwKey)
	hostnames = []gatewayv1beta1.Hostname{"foo-8080.com"}
	akogatewayapitests.UpdateHTTPRoute(t, httpRouteName, namespace, parentRefs, hostnames, nil)
	waitAndverify(t, key)

	// delete
	akogatewayapitests.TeardownHTTPRoute(t, httpRouteName, namespace)
	waitAndverify(t, key)
	akogatewayapitests.TeardownGateway(t, gatewayName, DEFAULT_NAMESPACE)
	waitAndverify(t, gwKey)
	akogatewayapitests.TeardownGatewayClass(t, gatewayClassName)
	waitAndverify(t, gwClassKey)
}
