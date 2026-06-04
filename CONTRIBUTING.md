# Contributing

Anyone is welcome to contribute, thank you!

Simple steps:

1. Clone the repo:

```bash
git clone https://github.com/your-username/your-repo.git
cd CF-contest-reminder-discord-bot
git checkout -b my-change
```

2. Make your changes locally.

3. Commit and push your branch:

4. Open a Pull Request and wait for review.

On the Pull Request please include:
- What you changed (clear bullet points).
- A brief explanation of *why* the change is needed.
- Whether you used AI tools and how they were used.

small contributions are welcome!

**Running Tests**
- **CI**: Tests run automatically on every push and pull request via the GitHub Actions workflow at `.github/workflows/go.yml`.
	- The workflow runs `go build` and `go test ./...` and will fail the run if tests fail.

- **Run tests locally**:

```bash
# run all tests in the repository
go test ./...

# run tests for the `bot` package with verbose output
go test ./bot -v

# run a single test by name in the `bot` package
go test -run TestFormatContestDiscordMessage ./bot -v
```

- **Before opening a Pull Request**: ensure `go test ./...` passes locally and push your branch so CI can run the same checks.
