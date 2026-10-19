
# Backend Rules

- Follow existing code patterns; do not add abstractions without a concrete need.
- Keep business logic inside its owning module.
- Treat API contracts and database migrations as compatibility boundaries.
- Validate input and enforce authorization on every protected operation.
- Use database transactions for multi-step operations that must succeed or fail together.
- Never trust client-supplied prices, payment status, stock availability, or user identity.
- Add or update tests for changed behavior.
- Never report a check as passed unless it was actually executed.
- Do not deploy or run destructive production operations without explicit authorization.
- Before completion, run the required CI checks and report failures honestly.
- 
