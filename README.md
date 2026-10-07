# MyGo's benchmarks

The results of MyGo's benchmarks, which .github/workflows/bench.yml of the
main branch measures on every push, written by scripts/bench.ts and shown at
https://mygo.egoist.dev/benchmarks.

- history/<yyyy-mm-dd>.json: every commit measured that day (UTC), with each OS's runner and results.
- latest.json: the last 300 commits, by series, which the website fetches.

Its commits skip CI ([skip ci]), the website's builds included.
