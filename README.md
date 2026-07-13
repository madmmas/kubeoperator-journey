# KubeOperator Journey

> **Companion code for the blog series: "From Zero to Kubernetes Operators with Kubebuilder"**
>
> Series: [From Zero to Kubernetes Operators with Kubebuilder](https://madmmasblog.vercel.app/series/from-zero-to-kubernetes-operators-with-kubebuilder/)

This repository contains working code examples organized by blog post.
Each tag matches a specific post so you can check out exactly the code state
for that point in the series.

## Series Structure

| Post | Tag | Topic |
|------|-----|-------|
| [Blog 1](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-01-why-operators-exist) | `blog-01` | Why Kubernetes Operators Exist — The Problem They Actually Solve |
| [Blog 2](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-02-control-loop) | `blog-02` | The Kubernetes Control Loop — Explained by Building One |
| [Blog 3](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-03-kubebuilder-scaffold) | `blog-03` | Kubebuilder From Zero — Scaffold, Structure, and Your First CRD |
| [Blog 4](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-04-first-reconciliation-loop) | `blog-04` | Your First Reconciliation Loop — Line by Line |
| ... | ... | ... |

## Prerequisites

| Blog | What you need |
|------|----------------|
| 1–2 | Go 1.21+ |
| 3 | Go + [Kubebuilder](https://book.kubebuilder.io/quick-start.html) |
| 4 | Above, plus a Docker runtime, `kind`, `kubectl`, and `make` to run against a local cluster |

Install steps for everything are in [Tooling setup](#tooling-setup) below.

## Running Blog 1

```bash
git checkout blog-01
go run ./cmd/why-operators
```

## Running Blog 2

```bash
git checkout blog-02
go run ./cmd/control-loop
```

## Blog 3 — scaffold reference

`blog-03` ships a **reference** `DatabaseCluster` types file (not a full Kubebuilder project):

```bash
git checkout blog-03
# Study: kubebuilder/api/v1alpha1/databasecluster_types.go
```

Generate the real scaffold locally (as the post walks through):

```bash
kubebuilder init --domain madmmas.dev --repo github.com/madmmas/kubeoperator-journey
kubebuilder create api --group databases --version v1alpha1 --kind DatabaseCluster --resource --controller
```

## Blog 4 — Reconcile implementation

`blog-04` adds the production-layout API types and a complete `Reconcile()` implementation:

```bash
git checkout blog-04
# Study:
#   api/v1alpha1/databasecluster_types.go
#   api/v1alpha1/groupversion_info.go
#   internal/controller/databasecluster_controller.go
# Example CR: internal/crd/database-cluster-example.yaml
```

This repo does **not** include a full Kubebuilder project (`Makefile`, manager `cmd/`, generated CRDs). To run against a cluster, generate the scaffold from Blog 3, then use these files as the types + controller implementation:

```bash
kind create cluster --name kubeoperator-dev
make generate manifests install
make run
# In another terminal:
kubectl apply -f internal/crd/database-cluster-example.yaml
kubectl get databasecluster -w
```

## Tooling setup

Commands below prefer Homebrew on macOS. Linux alternatives are noted where useful.

### 1. Go (Blog 1+)

```bash
# macOS
brew install go

# Linux (example)
# See https://go.dev/doc/install
```

Confirm:

```bash
go version   # need 1.21+ for this repo; Kubebuilder may want a newer Go — use the latest stable
```

Put Go binaries on your `PATH` (needed for `go install` tools):

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
# add that line to ~/.zshrc or ~/.bashrc
```

### 2. Git

```bash
brew install git          # macOS, if missing
# Linux: sudo apt install git   /   sudo dnf install git
git --version
```

### 3. Make (Blog 4 — `make generate`, `make run`, …)

Kubebuilder projects use a `Makefile`. On macOS, `make` comes with Xcode Command Line Tools:

```bash
xcode-select --install    # macOS, if `make` is missing
# Linux: sudo apt install build-essential
make --version
```

### 4. Container runtime — Rancher Desktop (Blog 4)

`kind` runs cluster nodes as containers, so it needs a Docker-compatible API. [Rancher Desktop](https://rancherdesktop.io/) provides that without Docker Desktop.

1. Install Rancher Desktop and open it once so the VM starts.
2. Preferences → Container Engine → **dockerd (moby)** → Apply (restart).  
   `kind` needs the Docker API; plain `containerd` is not enough for the usual workflow.
3. Optional: Preferences → Kubernetes → disable **Enable Kubernetes**.  
   Rancher’s built-in K3s is separate from `kind`. Turning it off frees CPU/RAM. Keep `kubectl` installed — Rancher Desktop still provides it.
4. Verify:

   ```bash
   docker version
   docker ps
   ```

**Alternatives:** Docker Desktop, or any engine that exposes a Docker API `kind` can use.

Troubleshooting (macOS): if pods inside kind fail DNS lookups, try Preferences → Virtual Machine → Emulation → **QEMU** instead of VZ, then recreate the cluster.

### 5. kubectl (Blog 4)

Often installed with Rancher Desktop. Otherwise:

```bash
brew install kubectl
# Linux: see https://kubernetes.io/docs/tasks/tools/
kubectl version --client
```

### 6. kind (Blog 4)

```bash
brew install kind
# or: go install sigs.k8s.io/kind@latest

kind version
kind create cluster --name kubeoperator-dev
kubectl cluster-info --context kind-kubeoperator-dev
```

Delete when finished:

```bash
kind delete cluster --name kubeoperator-dev
```

### 7. Kubebuilder (Blog 3+)

Official install ([Kubebuilder Quick Start](https://book.kubebuilder.io/quick-start.html)):

```bash
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/
kubebuilder version
```

On macOS you can also use `brew install kubebuilder` (community formula; the curl method above is what the project documents).

Scaffolding a project will pull related tools (`controller-gen`, `kustomize`, etc.) via the generated `Makefile` when you run `make` targets — you usually do not install those by hand.

### Quick verify

```bash
go version
docker version
kubectl version --client
kind version
kubebuilder version
make --version
```

## Repository Layout

```
kubeoperator-journey/
├── cmd/
│   ├── why-operators/                         # Blog 1 — the problem demo
│   └── control-loop/                          # Blog 2 — control loop simulation
├── kubebuilder/api/v1alpha1/                  # Blog 3 — reference types (study only)
├── api/v1alpha1/                              # Blog 4 — real Kubebuilder API path
│   ├── databasecluster_types.go
│   └── groupversion_info.go
└── internal/
    ├── problem/                               # Blog 1 — manual operator simulation
    ├── watcher/                               # Blog 2 — etcd analogue (store + watch)
    ├── reconciler/                            # Blog 2 — Watch→Enqueue→Reconcile pattern
    ├── controller/                            # Blog 4 — DatabaseCluster Reconcile()
    └── crd/                                   # Example DatabaseCluster YAML
```

## License

MIT
