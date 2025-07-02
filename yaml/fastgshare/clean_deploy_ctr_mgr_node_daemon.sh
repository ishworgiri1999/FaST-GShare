#!/bin/bash
current_dir=$(dirname "$0")
kubectl delete fastpods --all -n fast-gshare
kubectl delete -f ${current_dir}/fastgshare-node-daemon.yaml
kubectl delete -f ${current_dir}/fastpod-controller-manager.yaml
