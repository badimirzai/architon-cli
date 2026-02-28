# Contributing

Thanks for your interest in improving Architon CLI (rv).
This project aims to provide deterministic, auditable checks for robotics hardware specs.

---

## Legal Requirement (Read First)

By contributing, you confirm that you:

- License your contribution under Apache-2.0
- Agree to the terms in CLA.md
- Have the right to submit the code

---

## Scope

Contributions should fit at least one of these:

- New verification rules or safety checks based on real-world robotics constraints
- Improvements to error messages or diagnostics
- Onboarding improvements, documentation, or examples
- Performance improvements that maintain determinism
- Bug fixes with test coverage

#### Not in current scope for the core

**Rationale**:
The project is currently focused on deterministic, auditable checks.
Exploration of these areas may happen later only if they do not undermine that goal.

---
## Workflow

### Standard contribution path
Default path for all contributors:

1. Create an issue describing the need and proposed change.
2. Wait for Maintainer feedback and approval.
3. Create a feature branch from `main`.
4. Implement the change and add tests.
5. Submit a pull request.

This avoids misaligned work and wasted effort.

### For frequent contributors
If approved by the Maintainer, collaborator status may be granted.
This allows direct branch creation and reduces friction.

---

## Expectations

- Small, focused pull requests
- Include tests where meaningful
- Follow existing code patterns
- No breaking changes without discussion
- Deterministic behavior only

Run locally before opening a PR:

```bash
make verify
# or
go test ./...
go build ./...
```
----
## Commit style 
- Use clear prefixes:
  - `feat:` new feature
  - `fix:` bug fix
  - `docs:` documentation changes
  - `test:` tests only
  - `refactor:` structural changes without behavior change
- Keep messages brief and to the point.
----
## Final Note

This project is evolving. Expect rules to refine over time. Drive the project forward with clarity and discipline.
