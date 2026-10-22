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
  --el-color-primary: #2563eb;
  --el-color-primary-light-3: #60a5fa;
  --el-color-primary-light-5: #93bbfd;
  --el-color-primary-light-7: #bfdbfe;
  --el-color-primary-light-8: #dbeafe;
  --el-color-primary-light-9: #eff6ff;
  --el-color-primary-dark-2: #1d4ed8;
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
  .el-dialog { width: min(480px, calc(100vw - 24px)) !important; margin-top: 6vh !important; }
  .el-message-box { width: calc(100vw - 32px) !important; }
  .el-dialog__body { max-height: 72vh; overflow-y: auto; }
  .el-form-item { display: flex; flex-direction: column; align-items: stretch; }
  .el-form-item__label { width: auto !important; text-align: left; justify-content: flex-start; margin-bottom: 4px; line-height: 1.4; padding-right: 0; }
  .el-form-item__content { margin-left: 0 !important; width: 100%; }
  .el-form-item__content > .el-input,
  .el-form-item__content > .el-select,
  .el-form-item__content > .el-date-editor,
  .el-form-item__content > .el-input-number,
  .el-form-item__content > .el-cascader { width: 100%; }
  .el-form-item__content > .el-input-group,
  .el-form-item__content > .el-input__wrapper { width: 100%; }
}
</style>
