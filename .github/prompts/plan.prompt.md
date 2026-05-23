---
description: "scafctl-plugin-sdk: Create an implementation plan for a feature. Produces a structured blueprint with architecture decisions, task breakdown, interface design, and testing strategy."
agent: "planner"
argument-hint: "Describe the feature to plan (e.g., 'Add streaming output support to ProviderPlugin')"
---
Create a structured implementation blueprint for the described feature:

1. **Summary** -- What and why
2. **Architecture decisions** -- Packages affected, new types, interface changes, proto changes
3. **Task breakdown** -- Ordered steps with files, complexity, dependencies
4. **Interface design** -- Define contracts first
5. **Error handling** -- Sentinel errors, wrapping strategy
6. **Testing strategy** -- Unit tests, benchmarks, integration tests
7. **Documentation** -- README, examples, doc comments
8. **Risks & edge cases** -- What could go wrong, breaking changes, impact on scafctl

Follow scafctl-plugin-sdk conventions: lightweight dependencies, plugin-side only, interface stability.
