Project-wide TODO List

This document is an aggregated checklist of the outstanding TODO / FIXME /
XFAIL / SKIP comments detected in the current workspace.  Items are grouped
coarsely so that you can prioritise work without having to read through every
upstream dependency.

## 1  Local Go code (`internal/…`)
- [x] VWAP implementation  
  - Confirm that **time-window eviction**, **size-limit eviction** and
    **variance / std‐dev** logic are mathematically sound.
  - Ensure **concurrency safety** (all public methods must be goroutine-safe).
  - Add fast paths for empty input and zero volume.
  - Track **invalid/negative input** via `MetricsTracker.FeatureErrorsInc()`.

- [ ] Benchmarks  
  - Tighten benchmark setup (avoid allocation in hot loop).

## 2  Project-specific Python (none found)

_No TODO markers were found in Python files that belong to this project._

## 3  Third-party Python packages inside `venv/`
The virtual-environment contains **hundreds** of TODO markers originating from
upstream projects (`sympy`, `pandas`, `pytest`, `mpmath`, `google-protobuf`,
etc.).  These are **not actionable** for this repository unless you intend to
contribute upstream patches.  Typical categories:

* Unimplemented tests (`TODO: Restore tests once warnings are removed …`)
* Type-annotation glitches (`# TODO(typing): …`)
* Performance placeholders (`# TODO: optimise`)
* Feature gaps (`# TODO: Add support for …`)



---

_Last updated automatically; feel free to edit as tasks are completed._
