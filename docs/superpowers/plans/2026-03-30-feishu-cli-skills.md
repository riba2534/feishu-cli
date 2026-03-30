# feishu-cli Skills Expansion Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add official-aligned `feishu-cli-*` skills so the repository's AI skill surface is close to `larksuite/cli` without renaming existing skills.

**Architecture:** Keep the current granular skills intact, then add thin umbrella/family skills for the main product areas (`shared`, `doc`, `im`, `drive`, `sheets`, `calendar`, `task`, `contact`, `wiki`, `base`, `whiteboard`). Update project docs so the skill index reflects the expanded catalog.

**Tech Stack:** Markdown skill files, existing `feishu-cli` CLI commands, repository docs.

---

### Task 1: Add umbrella skills

**Files:**
- Create: `skills/feishu-cli-shared/SKILL.md`
- Create: `skills/feishu-cli-doc/SKILL.md`
- Create: `skills/feishu-cli-im/SKILL.md`
- Create: `skills/feishu-cli-drive/SKILL.md`
- Create: `skills/feishu-cli-sheets/SKILL.md`
- Create: `skills/feishu-cli-calendar/SKILL.md`
- Create: `skills/feishu-cli-task/SKILL.md`
- Create: `skills/feishu-cli-contact/SKILL.md`
- Create: `skills/feishu-cli-wiki/SKILL.md`
- Create: `skills/feishu-cli-base/SKILL.md`
- Create: `skills/feishu-cli-whiteboard/SKILL.md`

- [ ] **Step 1: Draft the skill files**
- [ ] **Step 2: Keep each skill thin and command-oriented**
- [ ] **Step 3: Make sure names and descriptions mirror the official lark-cli concept**

### Task 2: Update repository docs

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: Replace stale skill counts**
- [ ] **Step 2: Add the new skill names to the index/table**
- [ ] **Step 3: Keep the existing feishu-cli- skills listed as first-class entry points**

### Task 3: Verify the catalog

**Files:**
- Inspect: `skills/`
- Inspect: `README.md`
- Inspect: `CLAUDE.md`
- Inspect: `AGENTS.md`

- [ ] **Step 1: Confirm all new files exist**
- [ ] **Step 2: Confirm no broken relative links**
- [ ] **Step 3: Review the diff for consistency**

