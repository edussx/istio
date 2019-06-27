package lnputil

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"

	"istio.io/istio/pkg/test/util/retry"
	v1 "k8s.io/api/core/v1"
	extension_v1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func init() {
	rand.Seed(time.Now().Unix())
}

type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

const (
	VsPrefix       = "istio-lnp-vs"
	GwPrefix       = "istio-lnp-gw"
	SvcPrefix      = "istio-lnp-svc"
	PodPrefix      = "istio-lnp-pod"
	LnpNamespace   = "istio-lnp"
	IstioNamespace = "istio-system"
	MaxGoroutine   = 30
)

var (
	dynamicClient = GetDynamicClient("")
	kubeClient    = GetKubeClient("")
	gatewayGVR    = schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1alpha3",
		Resource: "gateways",
	}
	virtualServiceGVR = schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1alpha3",
		Resource: "virtualservices",
	}
)

type Setup interface {
	Render(int) error
	Purge(int) error
	GetSize() int
}

type IstioSetup struct {
	VsNumber      int32 // # of VirtualServices
	L7RuleNumber  int   // size of L7 rules
	TLSEnabled    bool
	ServiceNumber int
	matches       []interface{}
}

func (is *IstioSetup) GetSize() int {
	return int(is.VsNumber)
}

func (is *IstioSetup) Render(index int) error {
	var err error
	err = is.renderGw(index)
	if err != nil {
		return err
	}
	err = is.renderVs(index)
	if err != nil {
		return err
	}
	return nil
}

func (is *IstioSetup) getVsName(index int) string {
	return fmt.Sprintf("%s-%d", VsPrefix, index)
}

func (is *IstioSetup) getGwName(index int) string {
	return fmt.Sprintf("%s-%d", GwPrefix, index)
}

func (is *IstioSetup) renderGw(index int) error {
	gw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1alpha3",
			"kind":       "Gateway",
			"metadata": map[string]interface{}{
				"name":      is.getGwName(index),
				"namespace": LnpNamespace,
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"istio": "ingressgateway",
				},
				"servers": []interface{}{
					map[string]interface{}{
						"port": map[string]interface{}{
							"number":   80,
							"name":     "http",
							"protocol": "HTTP",
						},
						"hosts": []interface{}{"*"},
					},
				},
			},
		},
	}
	_, err := dynamicClient.Resource(gatewayGVR).Namespace(LnpNamespace).Create(gw, meta_v1.CreateOptions{})
	if err != nil {
		fmt.Printf("error while create Istio Gateway %s/%s\n", LnpNamespace, is.getGwName(index))
		return err
	}
	return nil
}

func (is *IstioSetup) updateRoute(reset bool) error {
	if len(is.matches) == 0 {
		return fmt.Errorf("empty `match` in virtualservice")
	}
	prefix := "/hello"
	if reset {
		prefix = "/fortio/fortio-0-l7-rule-0"
	}
	patchPayload := []patchStringValue{
		patchStringValue{Op: "replace", Path: "/spec/http/0/match/0/uri/prefix", Value: prefix},
	}
	patchBytes, _ := json.Marshal(patchPayload)
	_, err := dynamicClient.Resource(virtualServiceGVR).Namespace(LnpNamespace).Patch(is.getVsName(0), types.JSONPatchType, patchBytes, meta_v1.UpdateOptions{})
	return err
}

func (is *IstioSetup) renderVs(index int) error {
	matches := []interface{}{}
	for i := 0; i < is.L7RuleNumber; i++ {
		matches = append(matches, map[string]interface{}{
			"uri": map[string]interface{}{
				"prefix": fmt.Sprintf("/fortio/fortio-%d-l7-rule-%d", i, index),
			},
		})
	}
	is.matches = matches
	vs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1alpha3",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      is.getVsName(index),
				"namespace": LnpNamespace,
			},
			"spec": map[string]interface{}{
				"hosts":    []interface{}{"*"},
				"gateways": []interface{}{fmt.Sprintf("istio-lnp-gw-%d", index)},
				"http": []interface{}{
					map[string]interface{}{
						"match": matches,
						"route": []interface{}{
							map[string]interface{}{
								"destination": map[string]interface{}{
									"port": map[string]interface{}{
										"number": 80,
									},
									"host": fmt.Sprintf("%s-%d", SvcPrefix, rand.Intn(is.ServiceNumber)),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := dynamicClient.Resource(virtualServiceGVR).Namespace(LnpNamespace).Create(vs, meta_v1.CreateOptions{})
	if err != nil {
		fmt.Printf("error while create Istio VirtualService %s/%s\n", LnpNamespace, is.getVsName(index))
		return err
	}
	return nil
}

func (is *IstioSetup) Purge(index int) error {
	var err error
	err = is.purgeGw(index)
	if err != nil {
		return err
	}
	err = is.purgeVs(index)
	if err != nil {
		return err
	}
	return nil
}

func (is *IstioSetup) purgeGw(index int) error {
	err := dynamicClient.Resource(gatewayGVR).Namespace(LnpNamespace).Delete(is.getGwName(index), &meta_v1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (is *IstioSetup) purgeVs(index int) error {
	err := dynamicClient.Resource(virtualServiceGVR).Namespace(LnpNamespace).Delete(is.getVsName(index), &meta_v1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

type FortioServiceSetup struct {
	SvcNumber int32             // # of backend Kuberntes Services
	Labels    map[string]string // svc label
	Selector  map[string]string // pod selector
}

func (fss *FortioServiceSetup) GetSize() int {
	return int(fss.SvcNumber)
}

func (fss *FortioServiceSetup) getName(index int) string {
	return fmt.Sprintf("%s-%d", SvcPrefix, index)
}

func (fss *FortioServiceSetup) Render(index int) error {
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      fss.getName(index),
			Namespace: LnpNamespace,
			Labels:    fss.Labels,
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				v1.ServicePort{
					Name:     "http",
					Port:     80,
					Protocol: v1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   0,
						IntVal: 8080,
						StrVal: "",
					},
				},
			},
			Selector: fss.Selector,
		},
	}
	_, err := kubeClient.CoreV1().Services(LnpNamespace).Create(svc)
	if err != nil {
		return err
	}
	return nil
}

func (fss *FortioServiceSetup) Purge(index int) error {
	return kubeClient.CoreV1().Services(LnpNamespace).Delete(fss.getName(index), &meta_v1.DeleteOptions{})
}

type FortioDeploymentSetup struct {
	Name      string
	PodNumber int32
	Labels    map[string]string
}

func (fds *FortioDeploymentSetup) GetSize() int {
	return 1
}

func (fds *FortioDeploymentSetup) getName() string {
	return fmt.Sprintf("%s-%s", PodPrefix, fds.Name)
}

func (fds *FortioDeploymentSetup) Render(index int) error {
	deployment := &extension_v1beta1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "extensions/v1beta1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      fds.getName(),
			Namespace: LnpNamespace,
			Labels:    fds.Labels,
		},
		Spec: extension_v1beta1.DeploymentSpec{
			Replicas: &fds.PodNumber,
			Template: v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Labels: fds.Labels,
				},
				Spec: v1.PodSpec{
					NodeSelector: map[string]string{"role": "node"},
					Containers: []v1.Container{
						v1.Container{
							Image:           "hub.tess.io/istio/fortio:latest_release",
							ImagePullPolicy: v1.PullAlways,
							Name:            "fortio",
							Ports: []v1.ContainerPort{
								v1.ContainerPort{
									ContainerPort: 8080,
									Protocol:      v1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
	}
	_, err := kubeClient.ExtensionsV1beta1().Deployments(LnpNamespace).Create(deployment)
	if err != nil {
		return nil
	}
	return nil
}

func (fds *FortioDeploymentSetup) Purge(index int) error {
	deletionOptions := &meta_v1.DeleteOptions{}
	deltionProp := meta_v1.DeletePropagationBackground
	deletionOptions.PropagationPolicy = &deltionProp
	return kubeClient.ExtensionsV1beta1().Deployments(LnpNamespace).Delete(fds.getName(), deletionOptions)
}

func (fds *FortioDeploymentSetup) scale(number int) error {
	var (
		err           error
		replicaNumber = int32(number)
	)
	deployment, err := kubeClient.ExtensionsV1beta1().Deployments(LnpNamespace).Get(fds.getName(), meta_v1.GetOptions{})
	if err != nil {
		return err
	}
	deployment.Spec.Replicas = &replicaNumber
	_, err = kubeClient.ExtensionsV1beta1().Deployments(LnpNamespace).Update(deployment)
	return err
}

func GetIstioGatewayPodIP() (net.IP, error) {
	labelSelector := fmt.Sprintf("app=%s,istio=%s", "istio-ingressgateway", "ingressgateway")
	pods, err := kubeClient.CoreV1().Pods(IstioNamespace).List(meta_v1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	index := rand.Int() % len(pods.Items)
	ip := pods.Items[index].Status.PodIP
	return net.ParseIP(ip), nil
}

func GetFortioIPs(fortioSetup *FortioDeploymentSetup) ([]net.IP, error) {
	var err error
	labelSelector := labels.Set(fortioSetup.Labels).String()

	options := []retry.Option{retry.Timeout(2 * time.Minute), retry.Delay(10 * time.Second)}
	ips, err := retry.Do(func() (result interface{}, completed bool, err error) {
		ips := []net.IP{}
		pods, err := kubeClient.CoreV1().Pods(LnpNamespace).List(meta_v1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return nil, true, err
		}
		for _, pod := range pods.Items {
			if pod.Status.PodIP == "" {
				continue
			}
			if pod.Status.Phase == v1.PodPending {
				continue
			}
			ips = append(ips, net.ParseIP(pod.Status.PodIP))
		}
		if len(pods.Items) != len(ips) {
			fmt.Println("some pods are not ready; will retry")
			return ips, false, fmt.Errorf("Not all pods are ready; will retry")
		}
		return ips, true, nil
	}, options...)
	var ipList []net.IP
	if ips != nil {
		ipList = ips.([]net.IP)
	}
	if err != nil {
		return ipList, err
	}
	return ipList, nil
}

func CreateLnpResources(setup Setup) []error {
	token := make(chan struct{}, MaxGoroutine)
	errs := []error{}
	for i := 0; i < setup.GetSize(); i++ {
		token <- struct{}{}
		err := setup.Render(i)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func CleanLnpResources(setup Setup) []error {
	token := make(chan struct{}, MaxGoroutine)
	errs := []error{}
	for i := 0; i < setup.GetSize(); i++ {
		token <- struct{}{}
		err := setup.Purge(i)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

type TestCase struct {
	QPS         int
	Threads     int
	Time        time.Duration
	PayloadSize int
}

func RunControlPlaneLnp(clientSetup, serverSetup *FortioDeploymentSetup, testCase *TestCase) error {
	fortioClientIPs, err := GetFortioIPs(clientSetup)
	if err != nil {
		return err
	}
	fmt.Printf("fortio client pod IPs: %v\n", fortioClientIPs)
	// make sure server is also ready
	_, err = GetFortioIPs(serverSetup)
	if err != nil {
		return err
	}
	gatewayIP, err := GetIstioGatewayPodIP()
	if err != nil {
		return err
	}
	httpClient := &http.Client{}

	for _, fortioClientIP := range fortioClientIPs {
		url := fmt.Sprintf("http://%s:8080/fortio/", fortioClientIP.String())
		_, err = http.Get(url)
		if err != nil {
			return err
		}
		req, err := buildRequest(url, gatewayIP, testCase)
		if err != nil {
			// logs
			fmt.Printf("error while building request %v\n", err)
			return err
		}
		go func() {
			httpClient.Do(req)
		}()
	}

	return nil
}

func buildRequest(url string, gatewayIP net.IP, testCase *TestCase) (*http.Request, error) {
	if testCase.Threads == 0 {
		testCase.Threads = 16
	}
	if testCase.Time == 0 {
		// run the test until being interrupted
		testCase.Time = 0
	}
	if testCase.QPS == 0 {
		testCase.QPS = 3000
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		// logs
		return nil, err
	}
	q := req.URL.Query()
	q.Add("labels", "Fortio")
	q.Add("json", "on")
	q.Add("save", "on")
	q.Add("load", "Start")
	q.Add("runner", "http")
	q.Add("url", fmt.Sprintf("http://%s/fortio/fortio-0-l7-rule-0", gatewayIP.String()))
	q.Add("size", fmt.Sprintf("%d:100", testCase.PayloadSize))
	q.Add("qps", strconv.Itoa(testCase.QPS))
	q.Add("t", testCase.Time.String())
	q.Add("c", strconv.Itoa(testCase.Threads))
	q.Add("p", "50, 75, 90, 95, 99, 99.9")
	q.Add("r", "0.0001")
	req.URL.RawQuery = q.Encode()
	return req, nil
}

func RunEndpointDiscoveryTest(fds *FortioDeploymentSetup, timeout time.Duration, delay time.Duration) {
	var (
		options  = []retry.Option{retry.Timeout(timeout), retry.Delay(delay)}
		replicas = 1
	)
	fmt.Printf("running endpoint discovery test with delay=%s, timeout=%s\n", delay.String(), timeout.String())
	_, _ = retry.Do(func() (result interface{}, completed bool, err error) {
		err = fds.scale(replicas)
		if err != nil {
			fmt.Printf("unbale to scale Fortio deployment %s/%s\n; exit LnP test", LnpNamespace, fds.Name)
			return nil, true, err
		}
		replicas = int(math.Abs(float64(replicas - 1)))
		// run the test until timed out
		return nil, false, nil
	}, options...)
	fmt.Println("finish endpoint discovery test")
}

func RunRouteUpdateTest(is *IstioSetup, timeout time.Duration, delay time.Duration) {
	var (
		options = []retry.Option{retry.Timeout(timeout), retry.Delay(delay)}
		reset   = true
	)
	fmt.Printf("running route update test with delay=%s, timeout=%s\n", delay.String(), timeout.String())
	_, _ = retry.Do(func() (result interface{}, completed bool, err error) {
		err = is.updateRoute(reset)
		if err != nil {
			fmt.Printf("unbale to update route: %v\n; exit LnP test", err)
			return nil, true, err
		}
		// reset = !reset
		return nil, false, nil
	}, options...)
	fmt.Println("finish route update test")
}
