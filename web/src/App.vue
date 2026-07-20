<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <RouterView />
</template>

<style>
:root {
  --bg-deep: #060a13;
  --bg-card: #0d1424;
  --bg-hover: #131b2e;
  --accent: #06b6d4;
  --violet: #8b5cf6;
  --text: #e2e8f0;
  --text-muted: #94a3b8;
  --border: rgba(148, 163, 184, 0.1);
  --success: #22c55e;
  --warning: #f59e0b;
  --danger: #ef4444;
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: var(--bg-deep);
  color: var(--text);
  min-height: 100vh;
}

#app {
  min-height: 100vh;
}
</style>
