# upcloud-operator

UpCloud VM operator managing UpCloud Server instances on Kubernetes.

```bash
$ make generate manifest install
$ UPCLOUDOPERATOR_USERNAME=$UPCLOUD_USERNAME UPCLOUDOPERATOR_PASSWORD=$UPCLOUD_PASS ENABLE_WEBHOOKS=false make run
```

To create a sample VM:

```bash
$ kubectl apply -f ./config/samples/vm.yaml 
```
