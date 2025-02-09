package main

import (
	"flag"

	fastconfigurator "github.com/KontonGu/FaST-GShare/pkg/fast-configurator"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	klog "k8s.io/klog/v2"
)

var (
	ctr_mgr_ip_port string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		klog.Fatalf("Unable to initialize NVML: %v", nvml.ErrorString(ret))
		select {}
	}
	defer func() {
		ret := nvml.Shutdown()
		if ret != nvml.SUCCESS {
			klog.Fatalf("Unable to shutdown NVML: %v", nvml.ErrorString(ret))
		}
	}()

	fastconfigurator.Run(ctr_mgr_ip_port)

}

func init() {
	flag.StringVar(&ctr_mgr_ip_port, "ctr_mgr_ip_port", "fastpod-controller-manager-svc.kube-system.svc.cluster.local:10086", "The IP and Port to the FaST-GShare device manager.")
}
