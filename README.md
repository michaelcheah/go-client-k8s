# Building a Controller for Seldon Kubernetes Deployments with `client-go`

### Pre-Installation
Install `kubectl` following instructions [here](https://kubernetes.io/docs/tasks/tools/install-kubectl/)

This project was tested and deployed using `Minikube`. Instructions on how to install `Minikube` can be found [here](https://minikube.sigs.k8s.io/docs/start/)

Follow the instructions at [Seldon's official docs website](https://docs.seldon.io/projects/seldon-core/en/v1.1.0/workflow/quickstart.html) to install `seldon-core`. An alternative resource for understanding how the installation works can be found at [kataconda](https://www.katacoda.com/farrandtom/scenarios/seldon-core).

If all goes well, you should be able to see the following if the `seldon-core-operator` is installed correctly:
```bash
$ kubectl get deployments,svc,pods -n seldon-system 
NAME                                        READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/seldon-controller-manager   1/1     1            1           3h

NAME                             TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)   AGE
service/seldon-webhook-service   ClusterIP   10.110.107.206   <none>        443/TCP   3h

NAME                                             READY   STATUS    RESTARTS   AGE
pod/seldon-controller-manager-6cc549b9b9-wks7v   1/1     Running   0          3h

```

After this, one should be ready to use this repo.

### Instructions
`go.mod` and `go.sum` files have been provided and should provide easy compilation of the main binary.

Simply run 
```bash
go build main.go
```

This will produce a `main` binary executable that can be run as an application in your terminal. Otherwise, running `go run main.go` will work fine.

There are a few command line arguments that can be passed into the application (run `go run main.go --help` for more info).

The most important flags are `--kubeconfig` and `--config`. Specify the full path to your kubernetes config file (usually `$HOME/.kube/config`) with the `--kubeconfig` flag. You can specify the Seldon Deployment config file path with the `--config` flag.


### What does this application aim to do?
1. Reads and parses the provided config file at `--config` that is provided into `SeldonDeployment` instances (refer to the examples [`seldon_deployment.json`](seldon_deployment.json) and [`seldon_deployment_2.yaml`](seldon_deployment_2.yaml)). A Seldon Deployment is a Custom Resource and Seldon has provided Custom Resource Definitions in their [open source repository](https://github.com/SeldonIO/seldon-core/tree/master). 
2. Generates a golang client that can interact with the Custom Resource.
3. Creates the Custom Resource using the Kubernetes API.
4. Waits for the Custom Resource to be created and become available
5. Scales the resource to 2 replicas
6. Once the replicas are available, delete the Custom Resource.

Through steps 1-6, a logger will be running that captures descriptions emitted by the Kubernetes Custom Resouce until it is deleted.


## Progress
### Milestones achieved
* Successfully parsed the Custom Resource config files into `SeldonDeployment` instances
* Successfully generated a client that can send instructions to the Custom Resource
* Confirm the Custom Resource was created through the Informer
* Confirm the Custom Resource was created through `kubectl` (`watch kubectl get deployments -n seldon`)
* Confirm via Informers that the replica specs were updated
* Confirm the Custom Resource was deleted through the Informer + `kubectl`


### Milestones that weren't completed
* Confirm Custom resources can scale replica sets from 1 to 2 via the go clients
* Understand why the specs provided by the Informer and `kubectl` did not match when trying to scale replicas


## High Level Design Overview
The implementations have been split into one small and one larger package. A small package `parse` was created to test that the Custom Resource Definitions could be parsed properly when read from config files. In particular, a worry was that users might mix `json` and `yaml` files for configuration and it was found that `yaml` files had a tendency to misbehave since the CRD structs were tagged with `json` tags.

A larger package called `deployer` contains 4 main sections:
1. `deployer.go`: Implements the `Deployer`, which is the component responsible for controlling and keeping a reference to the kubernetes client, and applying instructions on the Custom Resource in a way that the next instruction is not called before the previous one has been deemed finished.
2. `observer.go`: Implements the `Observer`. This is a wrapper around the `Informer`/`InformerFactory` typically used by kubernetes go clients for event handling/monitoring of kubernetes resources. 
3. `instructions.go`: Implements the `DeploymentInstruction` interface. A key idea that this tries to capture is that an action by a kubernetes client is done in two stages 1. when executing the instruction, and 2. when the effect of the instruction has taken effect. Instructions that follow after each other should not be executed before the previous instruction is "done".
4. `colour.go`: Implements helper functions for colouring strings for terminal outputs. Purely aesthetic.


## What needs further improvement?
Besides the missed milestones above, a few other suggestions would greatly improve the quality of this project, given more time.
1. Implementing unit tests. Eventually, integration tests with [docker tests](https://github.com/ory/dockertest) could be useful. There are simple libraries already created by `kubernetes` that should help greatly with some good resources to [reference](https://itnext.io/testing-kubernetes-go-applications-f1f87502b6ef)
2. I realised a little too late I might have been re-inventing the wheel with the `Deployer` and `Observer`, and might have done better opting for a [custom controller approach](https://medium.com/speechmatics/how-to-write-kubernetes-custom-controllers-in-go-8014c4a04235)
3. I did quite like the idea of developing custom `DeploymentInstruction` methods, that can be built into an instruction set. Given more time, I might have attempted to make an application where instruction sets can be passed along as configs in yaml form and built dynamically by the application. 
4. Properly separating and abstracting away any coupling between the `Deployer` and `Observer`. They were designed to live in separate packages but due to lack of time I've left them in the same package. 
5. Add more flags/command line arguments for more control. 
6. I was not able to figure out how to get logs as detailed as one could get from running `kubectl describe`. This was quite frustrating.
7. I originally intended to deploy a simple model and test out this application myself (this would have required a new `DeploymentInstruction` that waited for prompts before deleting the deployment). Given more time, I think this would be quite cool to explore.

# Resources used (not organised)

https://medium.com/velotio-perspectives/extending-kubernetes-apis-with-custom-resource-definitions-crds-139c99ed3477

https://www.seldon.io/ibms-tom-farrand-creates-seldon-core-tutorial-on-katacoda/
https://www.katacoda.com/farrandtom/scenarios/seldon-core#

https://github.com/kubernetes/client-go/blob/v12.0.0/examples/create-update-delete-deployment/main.go

https://medium.com/programming-kubernetes/building-stuff-with-the-kubernetes-api-part-4-using-go-b1d0e3c1c899

https://github.com/vladimirvivien/k8s-client-examples/blob/master/go/pvcwatch-ctl/main.go

https://github.com/vladimirvivien/k8s-client-examples/blob/master/go/pvcwatch/main.go

https://medium.com/velotio-perspectives/extending-kubernetes-apis-with-custom-resource-definitions-crds-139c99ed3477

https://insujang.github.io/2020-02-11/kubernetes-custom-resource/

https://caylent.com/how-to-create-your-own-kubernetes-custom-resources

https://gist.github.com/mofelee/36b996d5c161dc60d551b52f3848a464

https://gianarb.it/blog/kubernetes-shared-informer

https://stackoverflow.com/questions/40975307/how-to-watch-events-on-a-kubernetes-service-using-its-go-client

https://engineering.bitnami.com/articles/a-deep-dive-into-kubernetes-controllers.html

