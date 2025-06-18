# FaST-GShare

## Introduction
**FaST-GShare: Enabling Efficient Spatio-Temporal GPU Sharing in Serverless Computing for Deep Learning Inference**. 

FaST-GShare is a Kubernetes-based GPU sharing mechanism that enables fine-grained resource allocation across both temporal and spatial dimensions. Users can allocate corresponding GPU computing resources by configuring the appropriate temporal quota_limit and quota_req, as well as spatial sm_partition and memory, within the annotations. More details, please refer to the [paper](https://dl.acm.org/doi/abs/10.1145/3605573.3605638).
![](https://github.com/KontonGu/FaST-GShare/blob/main/konton_test/architecture.png)
### FaSTPod 
FaSTPod is a Custom Resource with Controller that enables temporal and spacial GPU sharing. Users can specify fine-grained GPU resources for each Pod in the FaSTPod's YAML specification annotations, such as:
```
annotations: 
  fastgshare/gpu_quota_request: "0.7"
  fastgshare/gpu_quota_limit: "0.8"
  fastgshare/gpu_sm_partition: "30"
  fastgshare/gpu_mem: "2700000000"
```
Additionally, users can define the required replicas:
```
spec:
  replicas: 2
```
A sample FaSTPod deployment example is available in `yaml/fastpod/testfastpod.yaml`.



## Deployment

### Infrastruction Install
Install K8S infrastructure, NVIDIA Driver && Toolkit, and other prerequisite, please follow [Installation Guide](https://github.com/KontonGu/FaST-GShare/blob/main/install/README.md).

### Install FaST-GShare FaSTPod 
1. Deploy FaSTPod CRD (Custom Resource Definition)
    ```
    $ kubectl apply -f ./yaml/crds/fastpod_crd.yaml
    ```
2. Deploy FaSTPod Controller Manager and GPU Resource Configurator
    ```
    $ bash ./yaml/fastgshare/apply_deploy_ctr_mgr_node_daemon.sh
    ```
3. Test the FaSTPod example    
    ```
    $ kubectl apply -f ./yaml/fastpod/testfastpod.yaml
    ```
    check the FaSTPods and correponding Pods deployed:
    ```
    $ kubectl get pods -n fast-gshare
    $ kubectl get fastpods -n fast-gshare
    ```
4. Uninstall the FaST-GShare FaSTPod deployment
    ```
    $ bash ./yaml/fastgshare/clean_deploy_ctr_mgr_node_daemon.sh
    ```
---
### Install and Uninstall FaST-GShare-Function (without Autoscaler)
The deployment of the FaST-GShare-Function in this project does not include the FaST-GShare-Autoscaler and only deploys with a fixed number of replicas. The complete FaST-GShare serverless version should include the FaST-GShare-Autoscaler, and the basic verion of Autoscaler plugin can be found at [FaST-GShare-Autoscaler](https://github.com/KontonGu/FaST-GShare-Autoscaler.git).
- Install the FaST-GShare-Function Components
    ```
    $ bash ./install/install_fast-gshare-fn.sh
    ```
    Test if the FaST-GShare-Function is successfully deployed:
    ```
    $ kubectl apply -f yaml/fastpod/test-fastgshare-fn.yaml
    ```
- Uninstall the FaST-GShare-Function Components
    ```
    $ bash ./install/uninstall_fast-gshare-fn.sh
    ```

## Build FaST-GShare from Scratch based on the Code (for further Developing)
The detailed introduction to the FaST-GShare project's construction from the source code can be found in the `./develope` directory and [README](https://github.com/KontonGu/FaST-GShare/blob/main/develop/README.md).

