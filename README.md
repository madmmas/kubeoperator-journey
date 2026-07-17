# KubeOperator Journey

> **Companion code for the blog series:**
> [From Zero to Kubernetes Operators with Kubebuilder](https://madmmasblog.vercel.app)
>
> Build production-grade Kubernetes Operators from first principles — then evolve them into intelligent, LLM-powered agents.

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8?logo=go)](https://go.dev)
[![Kubebuilder](https://img.shields.io/badge/kubebuilder-3.14.0-326CE5?logo=kubernetes)](https://kubebuilder.io)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Blog](https://img.shields.io/badge/blog-madmmasblog.vercel.app-orange)](https://madmmasblog.vercel.app)

---

## What This Series Builds

Most operator tutorials start at `kubebuilder init` and leave you with generated code you don't fully understand. This series does it differently.

We start by building a **working Kubernetes-style control loop from scratch** — no k8s libraries, every component explicit and observable. Once you understand the pattern deeply, we let Kubebuilder generate the infrastructure and focus entirely on operator business logic. By the final phase, we evolve the operator into **KubeAgent** — a controller that uses LLM reasoning to make decisions no deterministic reconciler can.

```
Phase 1 — Foundations      Why operators exist + control loop internals
Phase 2 — Real Operator    Kubebuilder, CRDs, reconciliation, testing
Phase 3 — KubeAgent        Operators that observe, reason, and act
```

---

## Blog Posts & Code

Each post has a corresponding git tag. Check out any tag to see the exact code state for that post.

### Phase 1 — Foundations

| # | Post | Tag | Run |
|---|------|-----|-----|
| 1 | [Why Kubernetes Operators Exist](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-01-why-operators-exist) | `blog-01` | `go run ./cmd/why-operators` |
| 2 | [The Control Loop — Explained by Building One](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-02-control-loop) | `blog-02` | `go run ./cmd/control-loop` |
| 3 | [Kubebuilder From Zero — Scaffold, Structure, and Your First CRD](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-03-kubebuilder-scaffold) | `blog-03` | See post |
| 4 | [Your First Reconciliation Loop — Line by Line](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-04-first-reconciliation-loop) | `blog-04` | `make run` |

### Phase 2 — Building a Real Operator

| # | Post | Tag | Run |
|---|------|-----|-----|
| 5 | [Creating Resources From a CRD — Idempotency and the Reconcile Problem](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-2/blog-05-idempotency) | `blog-05` | `make run` |
| 6 | [Status, Conditions, and Observability — Designing CRDs That Operators Can Trust](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-2/blog-06-status-conditions) | `blog-06` | `make run` |
| 7 | [Finalizers — Safe Deletion and External Resource Cleanup](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-2/blog-07-finalizers) | `blog-07` | `make run` |
| 8 | [Why Is My Controller Not Reconciling? — Debugging Guide](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-2/blog-08-debugging) | `blog-08` | See post |
| 9 | [Testing Controllers — envtest, Fake Clients, What Actually Matters](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-2/blog-09-testing) | `blog-09` | `go test ./internal/controller/... -run Unit -v` |

### Phase 3 — KubeAgent *(coming soon)*

| # | Post | Tag |
|---|------|-----|
| 10 | The Operator Ceiling — What Rule-Based Controllers Can't Handle | `blog-10` |
| 11 | KubeAgent Architecture — Observe, Reason, Act | `blog-11` |
| 12 | Building the MVP — A Controller That Detects and Responds | `blog-12` |
| 13 | Connecting an LLM to Your Controller — Safely | `blog-13` |
| 14 | Shipping to Production — Helm, RBAC, Metrics, Leader Election | `blog-14` |

---

## Getting Started

### Blogs 1 & 2 — No cluster needed

```bash
git clone https://github.com/madmmas/kubeoperator-journey.git
cd kubeoperator-journey

# Blog 1: feel the pain of manual infrastructure management
git checkout blog-01
go run ./cmd/why-operators

# Blog 2: watch a control loop work in real time
git checkout blog-02
go run ./cmd/control-loop
```

### Blog 3 — Kubebuilder scaffold reference

```bash
git checkout blog-03
# Study the enriched types reference:
# kubebuilder/api/v1alpha1/databasecluster_types.go

# Generate the real scaffold locally (as the post walks through):
kubebuilder init --domain madmmas.dev --repo github.com/madmmas/kubeoperator-journey
kubebuilder create api --group databases --version v1alpha1 \
  --kind DatabaseCluster --resource --controller
```

### Blog 4 — First real Reconcile()

```bash
git checkout blog-04
kind create cluster --name kubeoperator-dev
make generate manifests install
make run
# In another terminal:
kubectl apply -f internal/crd/database-cluster-example.yaml
kubectl get databasecluster -w
```

### Blogs 5–7 — Real operator: Service, ConfigMap, Finalizer

```bash
git checkout blog-07   # includes all of Blogs 5 and 6
make generate manifests install
make run

# In another terminal — create a cluster with backup enabled:
kubectl apply -f - <<EOF
apiVersion: databases.madmmas.dev/v1alpha1
kind: DatabaseCluster
metadata:
  name: production-postgres
  namespace: default
spec:
  replicas: 3
  version: "15.4"
  storageSize: "1Gi"
  backupSchedule: "0 2 * * *"
  postgresConfig:
    max_connections: "200"
    shared_buffers: "256MB"
EOF

# Watch Phase transitions: Provisioning → Running
kubectl get databasecluster production-postgres -w

# Test upgrade detection (Blog 6)
kubectl patch databasecluster production-postgres \
  --type merge -p '{"spec":{"version":"15.5"}}'

# Test finalizer cleanup (Blog 7)
kubectl delete databasecluster production-postgres
# Operator deletes backup bucket before CR disappears
```

### Blog 8 — Debugging (no new operator code)

```bash
git checkout blog-08
# The diagnostics package documents the 5 most common failure modes.
# Use the kubectl commands in internal/diagnostics/checker.go
# when your controller isn't reconciling.
```

### Blog 9 — Unit tests

```bash
git checkout blog-09

# Run all unit tests (no cluster, no envtest needed)
go test ./internal/controller/... -run Unit -v
go test ./internal/backup/... -v

# Run integration tests (requires envtest setup)
# go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
# setup-envtest use 1.29 --bin-dir ./bin
# export KUBEBUILDER_ASSETS=$(setup-envtest use 1.29 -p path --bin-dir ./bin)
# go test ./internal/controller/... -tags=integration -v
```

---

## Repository Layout

```
kubeoperator-journey/
│
├── cmd/
│   ├── why-operators/          # Blog 1 — manual operator problem demo
│   └── control-loop/           # Blog 2 — control loop simulation (no k8s libs)
│
├── kubebuilder/api/v1alpha1/   # Blog 3 — enriched types reference (study only)
│
├── api/v1alpha1/               # Blog 4+ — real Kubebuilder-generated API path
│   ├── databasecluster_types.go
│   └── groupversion_info.go
│
└── internal/
    ├── problem/                # Blog 1 — ManualOperator simulation
    ├── watcher/                # Blog 2 — etcd analogue (Store + Watch)
    ├── reconciler/             # Blog 2 — Watch→Enqueue→Reconcile pattern
    ├── crd/                    # Example DatabaseCluster YAML
    ├── controller/             # Blog 4–9 — DatabaseCluster operator
    │   ├── databasecluster_controller.go   # Reconcile() — full Phase 2 logic
    │   ├── resources.go                    # Pure builder functions (Blog 5)
    │   └── databasecluster_controller_test.go  # 27 unit tests (Blog 9)
    ├── backup/                 # Blog 7 — S3 bucket simulation for finalizer demo
    │   └── bucket.go
    └── diagnostics/            # Blog 8 — operator debugging toolkit
        └── checker.go
```

---

## Tooling Setup

| Blog | Prerequisites |
|------|--------------|
| 1–2 | Go 1.21+ |
| 3 | Go + Kubebuilder 3.14+ |
| 4–9 | Go + Kubebuilder + Docker + `kind` + `kubectl` + `make` |

### Go

```bash
# macOS
brew install go
# Linux: https://go.dev/doc/install

go version   # need 1.21+
export PATH="$(go env GOPATH)/bin:$PATH"  # add to ~/.zshrc or ~/.bashrc
```

### Kubebuilder

```bash
curl -L -o kubebuilder \
  "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/
kubebuilder version
```

### Container runtime

`kind` needs a Docker-compatible API. [Rancher Desktop](https://rancherdesktop.io/) works without Docker Desktop:

1. Install and open Rancher Desktop
2. Preferences → Container Engine → **dockerd (moby)** → Apply
3. Optionally disable the built-in K3s (frees resources — kind provides its own cluster)

```bash
docker version && docker ps   # verify
```

### kind & kubectl

```bash
brew install kind kubectl
# or: go install sigs.k8s.io/kind@latest

kind create cluster --name kubeoperator-dev
kubectl cluster-info --context kind-kubeoperator-dev
```

### Quick verify

```bash
go version && kubebuilder version && kind version && kubectl version --client
```

---

## Contributing & Questions

Found a bug in an example? Question about a specific post?

- **Open an issue** tagged with the relevant `blog-XX` label
- **Start a discussion** for broader questions about the series direction
- **PRs welcome** for typos, broken code, or Go version compatibility fixes

Please include the blog post number in your issue title:
`[blog-07] finalizer not removed after bucket deletion`

---

## About the Author

**Moinuddin M Masud** — Senior Software & Data Engineer with 15+ years across fintech, telecom, and platform engineering. AWS certified (Data Engineering, Solutions Architect, Developer, Generative AI). CKAD.

- Blog: [madmmasblog.vercel.app](https://madmmasblog.vercel.app)
- GitHub: [@madmmas](https://github.com/madmmas)
- LinkedIn: [Moinuddin M Masud](https://linkedin.com/in/moinuddin-masud)

---

## License

MIT — see [LICENSE](LICENSE)

