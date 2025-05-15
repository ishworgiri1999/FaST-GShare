package main

import (
	"flag"

	fastconfigurator "github.com/KontonGu/FaST-GShare/internal/apps/fast-configurator"
	klog "k8s.io/klog/v2"
)

var (
	ctr_mgr_ip_port  string
	fastfunc_ip_port string
	mps              bool
)

func main() {
	klog.Infof("Starting FaST-GShare configurator main...")

	klog.InitFlags(nil)
	flag.Parse()
	klog.Info("CtrMgrIPPort: ", ctr_mgr_ip_port)
	klog.Info("Mps: ", mps)
	fastconfigurator.Run(ctr_mgr_ip_port, fastfunc_ip_port, mps)

}

func init() {
	flag.StringVar(&ctr_mgr_ip_port, "ctr_mgr_ip_port", "fastpod-controller-manager-svc.kube-system.svc.cluster.local:10086", "The IP and Port to the FaST-GShare device manager.")
	flag.StringVar(&fastfunc_ip_port, "fastfunc_ip_port", "fastfunc-svc.kube-system.svc.cluster.local:10088", "The IP and Port to the FaST-GShare function.")
	flag.BoolVar(&mps, "mps", false, "Enable MPS for the GPUs.")
}
