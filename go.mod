module github.com/kubernetes-app/redis-operator

go 1.15

require (
	github.com/go-redis/redis/v8 v8.8.2
	github.com/onsi/ginkgo v1.15.0
	github.com/onsi/gomega v1.10.5
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v0.19.2
	k8s.io/klog/v2 v2.8.0
	sigs.k8s.io/controller-runtime v0.7.0
)
