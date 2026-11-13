<script setup lang="ts">
withDefaults(defineProps<{ loading: boolean; error?: string; hasContent?: boolean; rows?: number; compact?: boolean }>(), { error: '', hasContent: false, rows: 3, compact: false })
defineEmits<{ retry: [] }>()
</script>

<template>
  <section class="content-state" :aria-busy="loading">
    <div v-if="loading && !hasContent" class="skeleton-list" :class="{ compact }" role="status" aria-label="正在加载内容">
      <span class="sr-only">正在加载内容，请稍候</span>
      <div v-for="n in rows" :key="n" class="skeleton-card" aria-hidden="true">
        <div class="skeleton-title"></div><div class="skeleton-meta"></div>
        <div class="skeleton-line"></div><div class="skeleton-line short"></div>
        <div v-if="!compact" class="skeleton-action"></div>
      </div>
    </div>
    <template v-else>
      <div v-if="error" class="load-error" role="alert"><span>{{ error }}</span><el-button link type="primary" @click="$emit('retry')">重新加载</el-button></div>
      <p v-else-if="loading" class="refresh-note" role="status">正在更新内容…</p>
      <slot v-if="!error || hasContent" />
    </template>
  </section>
</template>

<style scoped>
.content-state { min-width:0; }
.skeleton-list { display:grid; gap:16px; }
.skeleton-card { position:relative; overflow:hidden; padding:22px; border:1px solid var(--border,#e1e8ef); border-radius:12px; background:var(--bg-card,#fff); }
.skeleton-card::after { content:''; position:absolute; inset:0; transform:translateX(-100%); background:linear-gradient(100deg,transparent 20%,rgba(255,255,255,.8) 50%,transparent 80%); animation:skeleton-sweep 1.7s ease-in-out infinite; pointer-events:none; }
.skeleton-card > div { background:#eaf0f5; border-radius:5px; }
.skeleton-title { width:42%; height:19px; margin-bottom:12px; }
.skeleton-meta { width:26%; height:11px; margin-bottom:20px; }
.skeleton-line { width:94%; height:13px; margin-bottom:10px; }
.skeleton-line.short { width:66%; }
.skeleton-action { width:76px; height:28px; margin:18px 0 0 auto; }
.compact .skeleton-card { padding:18px; }
.compact .skeleton-meta { margin-bottom:12px; }
.compact .skeleton-line.short { display:none; }
.load-error { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:12px; padding:20px; border:1px solid var(--border); border-radius:10px; color:var(--text-secondary); }
.refresh-note { font-size:12px; color:var(--text-muted); margin-bottom:12px; }
.sr-only { position:absolute; width:1px; height:1px; overflow:hidden; clip-path:inset(50%); }
@keyframes skeleton-sweep { to { transform:translateX(100%); } }
@media(max-width:768px) { .skeleton-list { gap:12px; } .skeleton-card { padding:18px; } .skeleton-title { width:62%; } .skeleton-meta { width:42%; } }
@media(prefers-reduced-motion:reduce) { .skeleton-card::after { animation:none; display:none; } }
</style>
