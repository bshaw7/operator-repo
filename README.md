
# Kubernetes EC2 Operator

An OpenShift/Kubernetes Operator that manages AWS EC2 instances using Kubernetes Custom Resources. This allows you to provision and manage AWS infrastructure natively using standard YAML files.

## 📋 Prerequisites

Before deploying this operator, ensure you have the following:


1.  OpenShift / Kubernetes Cluster: Access to a running cluster.
2.  CLI Tools `oc` or `kubectl` installed and logged in.
3.  AWS Account An IAM User with `AmazonEC2FullAccess` permissions.
    You will need the **Access Key ID
    You will need the Secret Access Key
4.  Network Access Your cluster nodes must be able to pull images from the internal registry:
    `docker.io`
    (Note: If deploying to a cluster outside this network, push the image to a public registry like Docker Hub or Quay.io first).



## 🚀 Deployment Guide

Follow these steps to deploy the operator to any OpenShift cluster.

### Step 1: Clone the Repository
Get the manifests and configuration files to your local machine (or bastion host).



```bash
$ git clone https://github.com/bshaw7/operator-repo.git
$ cd operator-repo

```

### Step 2: Deploy the Operator

Install the CRDs (Custom Resource Definitions) and the Operator Deployment using the pre-built image.

Run this command from the project root:

```bash
$ oc apply -f dist/install.yaml

```

> **Note:** If you do not have `make` installed on the cluster machine, you can generate a raw YAML installer file locally using `kustomize build config/default > install.yaml` and then run `oc apply -f install.yaml`.

### Step 3: Configure AWS Credentials

The operator runs as a Pod in the `ec2operator-controller-manager` namespace. It needs your AWS credentials to talk to the EC2 API.

**We are using `ap-south-1` (Mumbai) for this setup.**

Run the following commands to create a Secret and inject it into the Deployment:

```bash
# 1. Create the Secret (Replace with your REAL keys)
oc create secret generic aws-creds \
  -n ec2operator-system \
  --from-literal=AWS_ACCESS_KEY_ID=YOUR_ACCESS_KEY_HERE \
  --from-literal=AWS_SECRET_ACCESS_KEY=YOUR_SECRET_KEY_HERE \
  --from-literal=AWS_REGION=ap-south-1

# 2. Inject the Secret into the Deployment environment
oc set env deployment/ec2operator-controller-manager \
  -n ec2operator-system \
  --from=secret/aws-creds

```

### Step 4: Verify Status

Check that the operator is running successfully.

```bash
oc get pods -n ec2operator-system

```

* **Status `Running**`: Ready to use.
* **Status `ImagePullBackOff**`: The cluster nodes cannot reach the private registry URL. Check your network or image pull secrets.

---

## 🛠 Usage

### 1. Create an EC2 Instance

Create a YAML file named `ec2-instance.yaml` with your specific AWS details (AMI and Subnet ID).

```yaml
apiVersion: compute.cloud.com/v1
kind: Ec2Instance
metadata:
  name: my-demo-server
spec:
  amiId: "ami-019715e023234"        # Change this to a valid AMI for your region
  region: "ap-south-1"
  keyPair: "myaws_keypair"    # replace with your keypair
  instanceType: "t2.large"   # instanfe type 
  subnet: "subnet-0d17e7ca2343455" # Change this to your valid Subnet ID
  # OPTIONAL FIELDS
  tags:
    Name: "my-demo-server"
    Environment: "Production"
    ManagedBy: "OpenShift Operator"


```

Apply it to the cluster:

```bash
oc apply -f ec2-instance.yaml

```

### 2. Verify Creation

Check the status of the object. Once the Operator processes it, the `STATUS` column will show the new AWS Instance ID.

```bash
oc get Ec2Instance my-demo-server -n ec2operator-system

```

You can also describe the object to see events:

```bash
oc describe Ec2Instance my-demo-server -n ec2operator-system

```

### 3. Delete the Instance

To terminate the AWS server, simply delete the Kubernetes manifest. The operator includes a **Finalizer**, so it will automatically clean up (terminate) the EC2 instance in AWS before removing the Kubernetes object.

```bash
oc delete -f ec2-instance.yaml

```

```bash
$ oc delete -f dist/install.yaml
```

---

## 🔧 Troubleshooting

If your instance is not being created, check the Operator logs:

```bash
oc logs -f deployment.apps/ec2operator-controller-manager -n ec2operator-system

```


```

```