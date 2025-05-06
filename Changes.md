/
├── internal/
│   └── apps/
│       ├── fast-configurator/
│       └── fastpod-controller-manager/
├── proto/
│   └── seti/ (gRPC service definitions)
│       └── v1/

Changed Files


In fast-configurator(daemon),
Added a service to run grpc server alongside the old one.
Added a service for provisioned gpu to manage mps service(creation and removal)


Removed  old publishing GPU on initialization. Moved file creation for fastPod configs creation from initialization when receving gpu configs from manager.


Instead of Pushing the gpu information, the manager should pull the information now as there are virtual gpus which are
dynamic in nature.


Initially, we query for all available gpus (physical) and virtual.
Virtual gpus are based on MIG profiles that allows the gpu to be split in slices.
For eg initially the profile looks like this. In total we can have possible virtual gpus of count 19.


+-----------------------------------------------------------------------------+
| GPU instance profiles:                                                      |
| GPU   Name             ID    Instances   Memory     P2P    SM    DEC   ENC  |
|                              Free/Total   GiB              CE    JPEG  OFA  |
|=============================================================================|
|   0  MIG 1g.5gb        19     7/7        4.75       No     14     0     0   |
|                                                             1     0     0   |
+-----------------------------------------------------------------------------+
|   0  MIG 1g.5gb+me     20     1/1        4.75       No     14     1     0   |
|                                                             1     1     1   |
+-----------------------------------------------------------------------------+
|   0  MIG 1g.10gb       15     4/4        9.75       No     14     1     0   |
|                                                             1     0     0   |
+-----------------------------------------------------------------------------+
|   0  MIG 2g.10gb       14     3/3        9.75       No     28     1     0   |
|                                                             2     0     0   |
+-----------------------------------------------------------------------------+
|   0  MIG 3g.20gb        9     2/2        19.62      No     42     2     0   |
|                                                             3     0     0   |
+-----------------------------------------------------------------------------+
|   0  MIG 4g.20gb        5     1/1        19.62      No     56     2     0   |
|                                                             4     0     0   |
+-----------------------------------------------------------------------------+
|   0  MIG 7g.40gb        0     1/1        39.38      No     98     5     0   |
|                                                             7     1     1   |
+-----------------------------------------------------------------------------+

If a request comes in asking for 100% ( 7 slices). The mig 7g.40gb should be selected and new available resoufce count is 0 as there would be no more possible other configs.

We just need to make sure we update the list of all virtual gpus(physical+virtual) are updated and sent on any request that accesses a gpu or releases it.




In fastpod-controller-manager,

Added annotations to support different allocation type fastpod default one that uses sm_partition,quota,
MPS that only uses memory( over subscription possible)

and Exclusive( using mig for allocation) that asks for sm count or sm partition( 100% means full slice. Slices may depend of gpu for eg a100 has max slice of 7).


Firstly, the server starts and listens to tcp connection.
On new connection it gets the hostname of the pod it runs on, and the node it runs on.
Saves the node. Creates a grpc connection to the node(daemon) for pulling gpu information.


It then adds it to available nodes, and also gets availableresources (virtual+physical)
which is then saved in the node.



Firstly the request is parsed to convert to PodRequest.
Then the podRequest is validated for required fields like for eg for Exclusive we need eithher smCount or smPercentage.

We then try to find best gpu and node


(currently on loop for each node) need to adapt this to database.

After that we ask for access to gpu if it's virtual for creation. 
if (mps or fastpod) we also send request for MPS server creation. From there we get log and pipe directory that can be attached to the pod for access.


On Removal, we remove the pod from podlist of the gpu if(mps,fastpod) else from exclusivePod attribute.
We then try to clean MPSserver if podlist length is 0 and release the virtual gpu.

