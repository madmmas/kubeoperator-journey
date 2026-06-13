# KubeOperator Journey

> **Companion code for the blog series: "From Zero to Kubernetes Operators with Kubebuilder"**
>
> Blog: [madmmasblog.vercel.app/blog/kubeoperator-journey](https://madmmasblog.vercel.app/blog/kubeoperator-journey)

This repository contains all working code examples, organized by blog post.
Each tag corresponds to a specific post so you can check out exactly the state
of the code at any point in the series.

## Series Structure

| Post | Tag | Topic |
|------|-----|-------|
| [Blog 1](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-01-why-operators-exist) | `blog-01` | Why Kubernetes Operators Exist |
| [Blog 2](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-02-control-loop) | `blog-02` | The Control Loop Explained |
| [Blog 3](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-03-kubebuilder-scaffold) | `blog-03` | Kubebuilder Scaffold From Zero |
| [Blog 4](https://madmmasblog.vercel.app/blog/kubeoperator-journey/phase-1/blog-04-first-reconciliation-loop) | `blog-04` | Your First Reconciliation Loop |
| ... | ... | ... |

## Running Blog 1 Code

```bash
git checkout blog-01
go run ./cmd/why-operators
```

## Running Blog 2 Code

```bash
git checkout blog-02
go run ./cmd/control-loop
```

**Prerequisites:** Go 1.21+. No Kubernetes cluster needed for Blog 1 or Blog 2.

## Repository Layout

```
kubeoperator-journey/
├── cmd/
│   ├── why-operators/      # Blog 1 — the problem demo
│   └── control-loop/       # Blog 2 — control loop simulation
├── internal/
│   ├── problem/            # Blog 1 — manual operator simulation
│   ├── watcher/            # Blog 2 — etcd analogue (store + watch)
│   ├── reconciler/         # Blog 2 — Watch→Enqueue→Reconcile pattern
│   └── crd/                # CRD YAML examples
└── docs/                   # Architecture diagrams
```

## License

MIT
