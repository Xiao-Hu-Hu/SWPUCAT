<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <RouterView />
</template>

<style>
:root {
  --bg-deep: #f4f4f5;
  --bg-card: #ffffff;
  --bg-hover: #f4f4f5;
  --accent: #2563eb;
  --accent-light: #3b82f6;
  --accent-bg: rgba(37, 99, 235, 0.08);
  --text: #18181b;
  --text-secondary: #3f3f46;
  --text-muted: #71717a;
  --border: #e4e4e7;
  --border-light: #f4f4f5;
  --success: #16a34a;
  --success-bg: rgba(22, 163, 74, 0.08);
  --warning: #d97706;
  --warning-bg: rgba(217, 119, 6, 0.08);
  --danger: #dc2626;
  --danger-bg: rgba(220, 38, 38, 0.08);
  --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05);
  --shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  --radius: 0.5rem;
  --radius-lg: 0.75rem;
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

/* Reserve the vertical scrollbar gutter at the document level so its width
   never toggles. Otherwise a page that overflows the viewport shows a
   page-level scrollbar that steals ~15px of width and pushes the whole UI
   left, and switching between tall and short pages causes a horizontal jump.
   `stable` keeps the same reserved gutter even when the page fits. */
html {
  scrollbar-gutter: stable;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans SC', sans-serif;
  background: var(--bg-deep);
  color: var(--text);
  min-height: 100vh;
  font-size: 14px;
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}

#app {
  min-height: 100vh;
}

/* Element Plus theme overrides */
:root {
  --el-color-primary: #096da9;
  --el-color-primary-light-3: #5399c3;
  --el-color-primary-light-5: #84b6d4;
  --el-color-primary-light-7: #b5d3e5;
  --el-color-primary-light-8: #cee2ee;
  --el-color-primary-light-9: #e6f0f6;
  --el-color-primary-dark-2: #075787;
  --el-color-success: #16a34a;
  --el-color-warning: #d97706;
  --el-color-danger: #dc2626;
  --el-color-info: #71717a;
  --el-bg-color: #ffffff;
  --el-bg-color-page: #f4f4f5;
  --el-text-color-primary: #18181b;
  --el-text-color-regular: #3f3f46;
  --el-text-color-secondary: #71717a;
  --el-text-color-placeholder: #a1a1aa;
  --el-border-color: #e4e4e7;
  --el-border-color-light: #f4f4f5;
  --el-fill-color-blank: #ffffff;
  --el-fill-color-light: #f4f4f5;
  --el-mask-color: rgba(24, 24, 27, 0.6);
  --el-menu-bg-color: #ffffff;
  --el-menu-text-color: #3f3f46;
  --el-menu-active-color: #2563eb;
  --el-menu-hover-bg-color: #f4f4f5;
}

/* Mobile global tweaks */
@media (max-width: 768px) {
  body { -webkit-tap-highlight-color: transparent; }
  .el-dialog { width: min(480px, calc(100vw - 24px)) !important; }
  .el-message-box { width: calc(100vw - 32px) !important; }
  .el-dialog__body { max-height: calc(100dvh - 180px); overflow-y: auto; }
  .el-button.is-circle { min-width: 40px; min-height: 40px; }
  .el-button.is-text.is-circle { min-width: 36px; min-height: 36px; }
  .el-form-item { display: flex; flex-direction: column; align-items: stretch; }
  .el-form-item .el-form-item__label { display: block; width: 100% !important; flex: 0 0 100% !important; text-align: left !important; justify-content: flex-start !important; margin-bottom: 6px; line-height: 1.4; padding-right: 0; }
  .el-form-item__content { margin-left: 0 !important; width: 100%; }
  .el-form-item__content > .el-input,
  .el-form-item__content > .el-select,
  .el-form-item__content > .el-date-editor,
  .el-form-item__content > .el-input-number,
  .el-form-item__content > .el-cascader { width: 100%; }
  .el-form-item__content > .el-input-group,
  .el-form-item__content > .el-input__wrapper { width: 100%; }
}
@media (max-width: 768px) {
  .el-table { font-size: 0.8125rem; }
  .el-table .cell { padding: 0 0.5rem; }
  .el-table .el-table__header th, .el-table .el-table__body td { padding: 0.5rem 0; }
  .el-table__body-wrapper { overflow-x: auto; -webkit-overflow-scrolling: touch; }
  .el-pagination { max-width: 100%; flex-wrap: wrap; justify-content: center; row-gap: 0.5rem; }
  .el-pagination__jump { display: none; }
}

/* Platform UI: scoped away from the public association homepage. */
.app-layout { --accent:#096da9; --accent-bg:#eaf4fb; --bg-deep:#f3f6f9; --border:#e1e8ef; --text:#203446; --text-muted:#677b8d; }
.app-layout > .main-area > .content { max-width:1680px; width:100%; margin-inline:auto; padding:24px; }
.app-layout .topbar .page-title { font-size:20px; }
.app-layout .el-card,.app-layout .card { box-shadow:none; }
.app-layout .el-table { --el-table-header-bg-color:#f6f8fb; font-variant-numeric:tabular-nums; }
.app-layout .el-empty { padding:24px 0; }
.app-layout .el-empty__description { margin-top:12px; }
.app-layout :is(.card-header,.page-header,.header) h2 { font-size:20px; }
:is(.app-layout,.login-page) :is(button,a,input):focus-visible { outline:2px solid #096da9; outline-offset:3px; }
:is(.app-layout,.login-page) .el-button { transition:background-color .18s,border-color .18s,box-shadow .18s; }
.app-layout .el-button:active:not(:disabled) { transform:translateY(1px); }
@media(max-width:768px) { .app-layout > .main-area > .content { padding:16px 12px; } .app-layout .topbar .page-title { font-size:16px; } }
@media(prefers-reduced-motion:reduce) { *,*::before,*::after { animation-duration:.01ms !important; transition-duration:.01ms !important; } }
</style>
