package lnputil

import (
	"testing"
)

var (
	is = &IstioSetup{
		VsNumber:        2,
		L7RuleSizes:     3,
		ServiceMaxIndex: 5,
	}
	fss = &FortioServiceSetup{
		SvcNumber: 5,
		Labels: map[string]string{
			"app":        "fortio",
			"fortioType": "server",
		},
		Selector: map[string]string{
			"app":        "fortio",
			"fortioType": "server",
		},
	}
	fds = &FortioDeploymentSetup{
		Name:      "server",
		PodNumber: 10,
		Labels: map[string]string{
			"app":        "fortio",
			"fortioType": "server",
		},
	}
)

func TestIstioRender(t *testing.T) {
	err := is.Render(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	return
}

func TestIstioPurge(t *testing.T) {
	err := is.Purge(0)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestSvcRender(t *testing.T) {
	err := fss.Render(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	return
}

func TestSvcPurge(t *testing.T) {
	err := fss.Purge(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	return
}

func TestPodRender(t *testing.T) {
	err := fds.Render(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	return
}

func TestPodPurge(t *testing.T) {
	err := fds.Purge(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	return
}
