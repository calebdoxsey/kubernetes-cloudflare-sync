# Kubernetes Cloudflare Sync

This App is intended to run in your Kubernetes Cluster on GKE and sync DNS records on Cloudflare with your nodes IPs.

## Example Usage
You can read this article to get an idea on why you would want to use it: http://www.doxsey.net/blog/kubernetes--the-surprisingly-affordable-platform-for-personal-projects

# Build
GKE provides you with a private cotainer image repository for each cluster.
The following two commands build the app and publish into your cluster-specific repository.

`docker build -t gcr.io/PROJECT_ID/kubernetes-cloudflare-sync:latest .`
`docker push gcr.io/PROJECT_ID/kubernetes-cloudflare-sync:latest`

# Configure

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-cloudflare-sync
  labels:
    app: kubernetes-cloudflare-sync
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubernetes-cloudflare-sync
  template:
    metadata:
      labels:
        app: kubernetes-cloudflare-sync
    spec:
      serviceAccountName: kubernetes-cloudflare-sync
      containers:
      - name: kubernetes-cloudflare-sync
        image: gcr.io/PROJECT_ID/kubernetes-cloudflare-sync
        args:
        - --dns-name=kubernetes.example.com
        env:
        - name: CF_API_KEY
          valueFrom:
            secretKeyRef:
              name: cloudflare
              key: api-key
        - name: CF_API_EMAIL
          valueFrom:
            secretKeyRef:
              name: cloudflare
              key: email
```
**Important:** Make sure to replace `PROJECT_ID`

The app needs to types of permissions:
1. talk to cloudflare and update DNS
2. get a list of nodes in the cluster and read their IP

The former requires just the API keys from cloudflare. We can store them as secret in the cluster by running:
`kubectl create secret generic cloudflare --from-literal=email='EMAIL' --from-literal=api-key='API_KEY'`

For the latter we create a `clusterrolebinding` in our cluster by running:
`kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user YOUR_EMAIL_ADDRESS_HERE`

**Important:** Make sure this is the same E-Mail as you use for running kubectl.

and then applying:

```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernetes-cloudflare-sync
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernetes-cloudflare-sync
rules:
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernetes-cloudflare-sync-viewer
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernetes-cloudflare-sync
subjects:
- kind: ServiceAccount
  name: kubernetes-cloudflare-sync
  namespace: default
```
