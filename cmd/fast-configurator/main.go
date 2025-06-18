package main

import (
	"flag"
	"os"
	"strconv"

	fastconfigurator "github.com/KontonGu/FaST-GShare/internal/apps/fast-configurator"
	klog "k8s.io/klog/v2"
)

var (
	ctr_mgr_ip_port  string
	fastfunc_ip_port string
	grpc_port        int
)

func main() {
	klog.Infof("Starting FaST-GShare configurator main...")

	klog.InitFlags(nil)
	flag.Parse()

	// Check environment variables and override if set
	if env := os.Getenv("CTR_MGR_IP_PORT"); env != "" {
		ctr_mgr_ip_port = env
	}
	if env := os.Getenv("FASTFUNC_IP_PORT"); env != "" {
		fastfunc_ip_port = env
	}
	if env := os.Getenv("GRPC_PORT"); env != "" {
		p, err := strconv.Atoi(env)
		if err != nil {
			grpc_port = p
		}
	}

	klog.Info("CtrMgrIPPort: ", ctr_mgr_ip_port)
	klog.Info("FastFuncIPPort: ", fastfunc_ip_port)
	fastconfigurator.Run(ctr_mgr_ip_port, fastfunc_ip_port, grpc_port)

}

func init() {
	flag.StringVar(&ctr_mgr_ip_port, "ctr_mgr_ip_port", "fastpod-controller-manager-svc.kube-system.svc.cluster.local:10086", "The IP and Port to the FaST-GShare device manager.")
	flag.StringVar(&fastfunc_ip_port, "fastfunc_ip_port", "fastfunc-controller-manager-svc.kube-system.svc.cluster.local:10088", "The IP and Port to the FaST-GShare function.")
	flag.IntVar(&grpc_port, "grpc_port", 5001, "The port to the gRPC server.")
}
