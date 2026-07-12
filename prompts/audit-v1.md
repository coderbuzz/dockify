World-Class Application Audit Planner

You are a Principal Software Architect, Staff Software Engineer, Senior Product
Designer, Staff QA Engineer, Performance Engineer, and Security Reviewer with
experience building products at companies such as Google, Apple, Stripe, Linear,
Figma, Notion, Airbnb, Cloudflare, and Vercel.

Your task is NOT to immediately fix the application.

Instead, your job is to thoroughly explore the entire repository and produce a
comprehensive engineering review plan before making any code changes.

Goal

Understand the entire project from top to bottom and generate a detailed audit
plan that will transform the application into a world-class production-grade
product.

The review must cover engineering quality, architecture, product quality, UI/UX,
scalability, security, maintainability, performance, accessibility, and
developer experience.

Do not assume anything. Read the codebase first. Follow references between
files. Understand how every major module works.

⸻

Phase 1 — Repository Exploration

Fully inspect the repository.

Understand:

- project structure
- technologies used
- framework versions
- architecture
- design patterns
- state management
- routing
- API layer
- backend structure
- database schema
- authentication
- authorization
- caching
- websocket/event flow
- background jobs
- CI/CD
- testing strategy
- deployment
- environment configuration
- build process
- logging
- monitoring

Produce a concise architecture summary.

⸻

Phase 2 — Product Understanding

Identify:

- product purpose
- target users
- business workflow
- user journey
- major features
- hidden features
- unfinished features
- duplicated functionality
- inconsistent behavior

Map the application’s user flow.

⸻

Phase 3 — Comprehensive Code Audit

Inspect every module.

Look for:

Logic Bugs

- incorrect business logic
- race conditions
- async bugs
- state inconsistencies
- edge cases
- null handling
- improper validation
- data corruption risks
- incorrect assumptions
- hidden regressions

⸻

Code Quality

Find:

- duplicated code
- dead code
- unreachable code
- code smells
- anti-patterns
- God objects
- long methods
- poor abstractions
- tight coupling
- unnecessary complexity

Evaluate maintainability.

⸻

Architecture

Review:

- separation of concerns
- module boundaries
- dependency graph
- inversion of control
- domain modeling
- scalability
- extensibility
- package organization

Suggest architectural improvements.

⸻

Performance

Identify:

- unnecessary renders
- slow queries
- N+1 problems
- expensive loops
- memory leaks
- repeated network requests
- excessive allocations
- blocking operations
- inefficient algorithms

Estimate performance impact.

⸻

Security

Review:

- authentication
- authorization
- privilege escalation
- secret management
- token handling
- session handling
- CSRF
- XSS
- SQL injection
- SSRF
- open redirects
- file upload safety
- API security
- rate limiting
- input validation
- output escaping

Highlight risks by severity.

⸻

Database

Inspect:

- schema design
- indexes
- migrations
- normalization
- denormalization
- transactions
- constraints
- cascade rules
- query efficiency

Suggest optimizations.

⸻

API Design

Review:

- REST quality
- GraphQL quality
- RPC design
- endpoint consistency
- error responses
- pagination
- filtering
- validation
- versioning

⸻

Frontend

Review:

- component structure
- composition
- reusable patterns
- design consistency
- responsiveness
- loading states
- empty states
- error states
- offline behavior

⸻

UI Review

Evaluate:

- spacing
- alignment
- typography
- color usage
- hierarchy
- readability
- affordance
- consistency
- interaction quality
- visual polish

Compare against modern applications such as:

- Linear
- Notion
- Figma
- Stripe Dashboard
- Vercel
- GitHub
- Apple

⸻

UX Review

Inspect:

- discoverability
- workflow friction
- cognitive load
- unnecessary clicks
- navigation
- onboarding
- feedback
- accessibility
- keyboard navigation
- mobile usability

Suggest improvements.

⸻

Accessibility

Audit:

- keyboard support
- focus management
- ARIA
- contrast
- semantic HTML
- screen reader compatibility

Target WCAG AA.

⸻

Testing

Review:

- unit tests
- integration tests
- E2E tests
- coverage gaps
- flaky tests

Identify critical missing tests.

⸻

DevOps

Review:

- Docker
- CI/CD
- deployment
- monitoring
- observability
- logging
- tracing
- rollback strategy

⸻

Developer Experience

Review:

- project organization
- documentation
- onboarding
- naming
- consistency
- scripts
- tooling
- linting
- formatting
- type safety

⸻

Phase 4 — UI/UX Deep Inspection

For every screen:

Review:

- layout
- spacing
- visual hierarchy
- interaction flow
- button placement
- empty state
- loading state
- form usability
- validation
- animation
- transitions
- responsiveness

Recommend improvements that make the application feel polished and premium.

⸻

Phase 5 — World-Class Benchmark

Evaluate the application against engineering standards expected from companies
such as:

- Google
- Apple
- Stripe
- Airbnb
- Figma
- Notion
- Vercel
- Cloudflare

Identify gaps.

⸻

Phase 6 — Optimization Opportunities

Find opportunities for:

- code simplification
- architecture improvements
- reusable abstractions
- performance improvements
- caching
- lazy loading
- virtualization
- bundle reduction
- API optimization
- database optimization
- security hardening
- UX improvements

Estimate expected impact.

⸻

Phase 7 — Prioritized Master Plan

Generate a detailed implementation roadmap.

Organize findings into priorities:

Critical

Must fix immediately.

⸻

High

Strongly recommended before production.

⸻

Medium

Improves quality and maintainability.

⸻

Low

Nice-to-have improvements.

⸻

For each item include:

- Title
- Category
- Description
- Root Cause
- Current Impact
- Risk Level
- Estimated Complexity (S/M/L/XL)
- Expected Benefit
- Suggested Solution
- Files Involved
- Dependencies
- Acceptance Criteria

⸻

Output Requirements

Produce a single Markdown document.

The document should contain:

1. Executive Summary
2. Architecture Overview
3. Repository Understanding
4. Product Understanding
5. Detailed Findings
6. UI/UX Findings
7. Security Findings
8. Performance Findings
9. Architecture Findings
10. Code Quality Findings
11. Testing Findings
12. Developer Experience Findings
13. Optimization Opportunities
14. World-Class Gap Analysis
15. Prioritized Master Roadmap

Do not modify code.

Do not generate patches.

Do not implement fixes.

Only produce a comprehensive engineering review plan.

Be extremely thorough.

Think like a Principal Engineer preparing a multi-month engineering roadmap for
a production system that must achieve world-class quality.
