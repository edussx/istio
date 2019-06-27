package lnp

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"istio.io/istio/tests/e2e/ebay/lnp/lnputil"
)

func TestLnp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lnp Suite")
}

var (
	timeout  = 5 * time.Minute
	delay    = 45 * time.Second
	testCase = &lnputil.TestCase{
		QPS:         3000,
		Threads:     16,
		Time:        timeout,
		PayloadSize: 0,
	}
	istioSetup = &lnputil.IstioSetup{
		VsNumber:      1,
		L7RuleNumber:  1,
		ServiceNumber: 1,
	}
	serviceSetup = &lnputil.FortioServiceSetup{
		SvcNumber: 1,
		Labels: map[string]string{
			"app":        "fortio-client",
			"fortioType": "server",
		},
		Selector: map[string]string{
			"app":        "fortio-server",
			"fortioType": "server",
		},
	}
	caseSetup = []lnputil.Setup{istioSetup, serviceSetup}
)

var _ = BeforeSuite(func() {
	for _, setup := range caseSetup {
		errors := lnputil.CreateLnpResources(setup)
		if len(errors) > 0 {
			fmt.Println(errors)
			return
		}
	}
})

var _ = AfterSuite(func() {
	for _, setup := range caseSetup {
		errors := lnputil.CleanLnpResources(setup)
		if len(errors) > 0 {
			continue
		}
	}
})

var _ = Describe("Istio LnP", func() {
	defer GinkgoRecover()

	Describe("Control plane LnP", func() {
		Context("with 1 VirtualService with 1 L7 rule with 1 svc and 1 pod for each svc", func() {

			var (
				fortioClient = &lnputil.FortioDeploymentSetup{
					Name:      "client",
					PodNumber: 1,
					Labels: map[string]string{
						"app":        "fortio-client",
						"fortioType": "client",
					},
				}
				fortioServer = &lnputil.FortioDeploymentSetup{
					Name:      "server",
					PodNumber: 1,
					Labels: map[string]string{
						"app":        "fortio-server",
						"fortioType": "server",
					},
				}
				fortioSetup = []lnputil.Setup{fortioClient, fortioServer}
			)

			BeforeEach(func() {
				fmt.Println("setting up fortio clients and servers")
				for _, setup := range fortioSetup {
					_ = lnputil.CleanLnpResources(setup)
					errors := lnputil.CreateLnpResources(setup)
					if len(errors) > 0 {
						fmt.Printf("got errors during setting up: %+v\n", errors)
						return
					}
				}
				// run test case
				err := lnputil.RunControlPlaneLnp(fortioClient, fortioServer, testCase)
				if err != nil {
					fmt.Printf("error while running lnp test case: %v\n", err)
				}
			})

			AfterEach(func() {
				fmt.Println("tearing down fortio clients and servers")
				for _, setup := range fortioSetup {
					errors := lnputil.CleanLnpResources(setup)
					if len(errors) > 0 {
						fmt.Printf("got errors during tearing down: %+v\n", errors)
						return
					}
				}
			})

			It("should watch the delay of endpoint discovery", func() {
				lnputil.RunEndpointDiscoveryTest(fortioServer, timeout, delay)
			})

			It("should watch the delay of updating L7 rule", func() {
				lnputil.RunRouteUpdateTest(istioSetup, timeout, delay)
			})

		})

	})

})
