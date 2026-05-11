# Backend CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

A starter Kanban board web app used as a teaching example. Two independently-runnable apps in a monorepo:

- `backend/` — Go REST API (Gin) backed by SQLite via `database/sql` + `sqlx`
- `frontend/` — Vue 3 (Vite, `<script setup>` Composition API) SPA that consumes the API

The target design (API contract, schema, conventions) lives in `docs/spec.md`. The phased implementation plan is in `docs/plan.md` (K1–K4). Read `docs/spec.md` before making non-trivial changes — it is the source of truth for the target API and data model.
