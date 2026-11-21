
## Understand the Project First
- I am continuing with a previous project.  You will find the summaries in SUMMARY.md files located in the top level as various sub folders.  NEXTSTEPS.md is used to note what has been completed and what are next steps in our roadmap.  Thorougly understand it and give me a recap so we can continue where we left off.

## Coding Style and Conservativeness
- Be conservative on how many comments are you are adding or modifying unless it is absolutely necessary (for example a comment could be contradicting what is going on - in which case it is prudent to modify it).  
- When modifying files just focus on areas where the change is required instead of diving into a full fledged refactor.
- Make sure you ignore 'gen' and 'node_modules' as it has a lot of files you wont need for most things and are either auto generated or just package dependencies
- When updating .md files and in commit messages use emojis and flowerly languages sparingly.  We dont want to be too grandios or overpromising.
- Make sure the playwright tool is setup so you can inspect the browser when we are implementing and testing the Dashboard features.
- Do not refer to claude or anthropic or gemini in your commit messages
- Do not rebuild the server - it will be continuosly be rebuilt and run by the air configs.  Output of the server will be written to /tmp/sdlserver.log.  Build errors will also be shown in this log file.
- Find the root cause of an issue before figuring out a solution.  Fix problems.
- Do not create workarounds for issues without asking.  Always find the root cause of an issue and fix it.
- The web module automatically builds when files are changed - DO NOT run npm build or npm run build commands.
- Proto files are automatically regenerated when changed - DO NOT run buf generate commands.

## Test Instructions
- Run tests with `make test`. CI (`.github/workflows/test.yml`) runs the same target on Linux.
- CI runs on Linux, where `sh` is dash and file watching uses inotify. Shell syntax, process
  handling and watcher timing can pass on macOS and fail there (PR 14). To reproduce a Linux-only
  failure, run the tests in a `golang:1.26` container as a non-root user with no git config.
- Tests that need git commits must set a git identity themselves. CI has none (see
  `dagbump/dagbump_test.go` TestMain).

## Releases
- Cut releases with `dagbump release --version vX.Y.Z --notes notes.md`, which follows
  `.dagbump.yaml`: it bumps `VERSION`, adds the notes to `RELEASE_NOTES.md`, tags, pushes and
  creates the GitHub release. Add `--dry-run` first.
- `make install` rewrites the tracked `.LAST_BUILT_AT`, and dagbump refuses to release with
  uncommitted changes. Restore the file with `git checkout .LAST_BUILT_AT` before releasing.
- `go.mod` pins `toolchain go1.26.x` so CI and govulncheck use a patched standard library. Raise
  it when vulncheck reports standard library advisories (issue 17).

## Summary instructions

- When you are using compact, please focus on test output and code changes

- For the ROADMAP.md always use the top-level ./ROADMAP.md so we have a global view of the roadmap instead of being fragemented in various folders.

## Session Workflow Memories
- When you checkpoint update all relevant .md files with our latest understanding, statuses and progress in the current session and then commit.

