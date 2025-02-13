package main

import (
	"flag"

	fastconfigurator "github.com/KontonGu/FaST-GShare/pkg/fast-configurator"
	klog "k8s.io/klog/v2"
)

var (
	ctr_mgr_ip_port string
)

func main() {
	klog.Infof("Starting FaST-GShare configurator main...")

	klog.InitFlags(nil)
	flag.Parse()

	fastconfigurator.Run(ctr_mgr_ip_port)

}

func init() {
	flag.StringVar(&ctr_mgr_ip_port, "ctr_mgr_ip_port", "fastpod-controller-manager-svc.kube-system.svc.cluster.local:10086", "The IP and Port to the FaST-GShare device manager.")
}
