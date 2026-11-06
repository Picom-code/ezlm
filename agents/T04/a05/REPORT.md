# T04 a05 report: processor-only laptops, 8 to 64 GB RAM

Slice: CPU-only laptops with 8 to 64 GB of RAM.

Deliverables:

- `internal/fit/profiles_cpu_test.go`: 16 pinned profiles, the plan table
  assertions, per-plan invariants, and a 28-machine free-memory grid.
- `agents/T04/a05/plans.md`: the plan table, invariants, findings with
  file:line, and open questions. Read that file first.

Summary: no plan in this slice exceeded its budget and no planner bug was
found that a test could prove. Three minor items are recorded in plans.md:
processor bandwidth is only measured on macOS (`internal/scan/scan.go:91`),
CPU plans do not reserve prompt-batch working memory (open question), and
lazy lookup tables reserve no page cache on CPU (open question). Nothing in
`internal/fit/` was changed.

Run: `go test -p 2 -run CPUOnly ./internal/fit/` (about 0.6 s). Regenerate
the table with `SHOW=1 go test -run ShowCPU -v ./internal/fit/`.
