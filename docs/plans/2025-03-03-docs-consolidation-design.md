# Documentation Consolidation Design

**Date:** 2025-03-03
**Goal:** Consolidate ONBOARDING_SUMMARY.md, ONBOARDING.md, and README.md into a single, concise, user-first README.

## Decision

**Approach:** Streamlined User README (~150-200 lines)

## Target Structure

```
README.md (~150-200 lines)
├── Header - Title, one-line description
├── Quick Start - Prerequisites, install, run, basic usage
├── Key Features - Bullet list of main capabilities
├── API Reference - Essential endpoints with examples
├── Configuration - Most common settings only
├── Testing - Basic test commands
├── Production Checklist - Security and deployment
└── Documentation Links - Point to docs/ for developer details

docs/developer-guide.md (new)
├── Architecture Deep Dive
├── Request Flow Diagrams
├── Component Details
├── Development Setup
├── Contributing Guidelines
└── Troubleshooting Guide
```

## Content Distribution

### Keep in README.md (User-Facing)
- All API endpoints and examples
- Essential configuration options
- Quick start guide
- Production security checklist
- Links to additional resources

### Move to docs/developer-guide.md (Developer-Facing)
- Architecture diagrams from ONBOARDING.md
- Request flow documentation
- Component details (storage layer, API layer, queue system)
- Development workflow details
- Contributing guidelines
- Troubleshooting section
- Common tasks reference

### Remove Entirely
- ONBOARDING_SUMMARY.md - dated review metadata, no longer relevant

## Files Changed

| File | Action |
|------|--------|
| README.md | Rewrite to streamlined user-first version |
| ONBOARDING.md | Delete (content moved to docs/developer-guide.md) |
| ONBOARDING_SUMMARY.md | Delete (dated, no longer needed) |
| docs/developer-guide.md | Create new (consolidated developer content) |

## Success Criteria
- Single README.md under 200 lines
- All user-facing content preserved in README
- All developer content preserved in docs/developer-guide.md
- No information lost, just reorganized
