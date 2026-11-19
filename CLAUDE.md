# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

This repository is a fresh scaffold: it contains only `README.md` ("First app is used Claude Code to develop") and an MIT `LICENSE`. No language, framework, build system, test runner, or linter has been chosen yet, so there are no build/lint/test commands to run.

The approved plan (Vietnamese) is in `docs/PLAN.md`: a website plus a POS system for a 5-branch café chain with a central warehouse, built with a Next.js + Tailwind front end, a Go (Gin) API and PostgreSQL. Test cases, which must be approved by the owner before any code is written, live in `docs/test-cases/`. Workflow: plan → test cases → owner approval → write tests → implement until tests pass.

When the first real code is added, update this file with:
- The commands to install dependencies, build, lint, run the full test suite, and run a single test.
- A short description of the high-level architecture (how the main modules fit together), once there is more than one module.
