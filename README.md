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

- **Blog 1–2:** Go 1.21+. No Kubernetes cluster needed.
- **Blog 3:** [Kubebuilder](https://book.kubebuilder.io/quick-start.html) (to generate the scaffold locally).
- **Blog 4:** A local Kubebuilder project (from Blog 3), plus `kind` and `kubectl` if you want to run the operator against a cluster.

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
