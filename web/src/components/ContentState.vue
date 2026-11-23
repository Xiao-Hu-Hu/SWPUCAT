<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ loading: boolean; error?: string; hasContent?: boolean; rows?: number; compact?: boolean }>(), { error: '', hasContent: false, rows: 3, compact: false })
defineEmits<{ retry: [] }>()

const showFeedback = ref(false)
const hasResolved = ref(!props.loading && !props.error)
const revealContent = ref(false)
const keepContent = computed(() => props.hasContent || hasResolved.value)
let feedbackTimer: ReturnType<typeof setTimeout> | undefined
let initialRequest = false

function clearFeedbackTimer() {
  clearTimeout(feedbackTimer)
  feedbackTimer = undefined
}

// Reserve the final layout immediately, but skip visual feedback for fast requests.
// Starting on mount also avoids creating timers during server rendering.
onMounted(() => {
  watch(() => props.loading, (loading) => {
    clearFeedbackTimer()
    if (loading) {
      initialRequest = !keepContent.value
      showFeedback.value = false
      revealContent.value = false
      feedbackTimer = setTimeout(() => { showFeedback.value = true }, 180)
    } else {
      revealContent.value = showFeedback.value && initialRequest && !props.error
      showFeedback.value = false
      if (!props.error) hasResolved.value = true
    }
  }, { immediate: true })
})
onBeforeUnmount(clearFeedbackTimer)
</script>

<template>
  <section class="content-state" :class="{ 'content-reveal': revealContent }" :aria-busy="loading">
    <div v-if="loading && !keepContent" class="skeleton-list" :class="{ compact, custom: !!$slots.skeleton, 'feedback-pending': !showFeedback }" role="status" aria-label="正在加载内容">
      <span class="sr-only">正在加载内容，请稍候</span>
      <slot name="skeleton">
      <div v-for="n in rows" :key="n" class="skeleton-card" aria-hidden="true">
        <div class="skeleton-title"></div><div class="skeleton-meta"></div>
        <div class="skeleton-line"></div><div class="skeleton-line short"></div>
        <div v-if="!compact" class="skeleton-action"></div>
      </div>
      </slot>
    </div>
    <template v-else>
      <div v-if="error" class="load-error" role="alert"><span>{{ error }}</span><el-button link type="primary" @click="$emit('retry')">重新加载</el-button></div>
      <p v-else-if="loading && showFeedback" class="refresh-note" role="status">正在更新内容…</p>
      <slot v-if="!error || keepContent" />
    </template>
  </section>
</template>

<style scoped>
.content-state { min-width:0; position:relative; }
.skeleton-list { display:grid; gap:16px; }
.skeleton-list.custom { display:block; }
/* Keep the measured card geometry without painting a flash of placeholders. */
.feedback-pending > :not(.sr-only) { visibility:hidden; }
.feedback-pending :deep(.skeleton-shell::after), .feedback-pending .skeleton-card::after { animation:none; }
.content-reveal { animation:content-reveal .14s ease-out; }
@keyframes content-reveal { from { opacity:.65; } to { opacity:1; } }
.skeleton-card { position:relative; overflow:hidden; padding:22px; border:1px solid var(--border,#e1e8ef); border-radius:12px; background:var(--bg-card,#fff); }
.skeleton-card::after { content:''; position:absolute; inset:0; transform:translateX(-100%); background:linear-gradient(100deg,transparent 20%,var(--skeleton-glow) 50%,transparent 80%); animation:skeleton-sweep 1.7s ease-in-out infinite; pointer-events:none; }
.skeleton-card > div { background:var(--skeleton-block); border-radius:5px; }
.skeleton-title { width:42%; height:19px; margin-bottom:12px; }
.skeleton-meta { width:26%; height:11px; margin-bottom:20px; }
.skeleton-line { width:94%; height:13px; margin-bottom:10px; }
.skeleton-line.short { width:66%; }
.skeleton-action { width:76px; height:28px; margin:18px 0 0 auto; }
.compact .skeleton-card { padding:18px; }
.compact .skeleton-meta { margin-bottom:12px; }
.compact .skeleton-line.short { display:none; }
.load-error { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:12px; padding:20px; border:1px solid var(--border); border-radius:10px; color:var(--text-secondary); }
.refresh-note { position:absolute; top:6px; right:8px; z-index:2; margin:0; padding:3px 8px; border-radius:6px; background:var(--bg-card,#fff); box-shadow:0 1px 5px #20344614; font-size:12px; color:var(--text-muted); pointer-events:none; }
.sr-only { position:absolute; width:1px; height:1px; overflow:hidden; clip-path:inset(50%); }
@keyframes skeleton-sweep { to { transform:translateX(100%); } }
@media(max-width:768px) { .skeleton-list { gap:12px; } .skeleton-card { padding:18px; } .skeleton-title { width:62%; } .skeleton-meta { width:42%; } }
@media(prefers-reduced-motion:reduce) { .content-reveal { animation:none; } .skeleton-card::after { animation:none; display:none; } }

/* Skeletons use each page's real card classes; only the contents are placeholders. */
:global(.skeleton-shell) { position:relative; overflow:hidden; pointer-events:none; }
:global(.skeleton-shell::after) { content:''; position:absolute; inset:0; transform:translateX(-100%); background:linear-gradient(100deg,transparent 20%,var(--skeleton-glow) 50%,transparent 80%); animation:skeleton-sweep 1.7s ease-in-out infinite; pointer-events:none; }
:global(.sk-block) { display:block; background:var(--skeleton-block); border-radius:4px; }
:global(.sk-title) { width:55%; height:24px; }
:global(.sk-meta) { width:160px; max-width:100%; height:19.2px; }
:global(.sk-line) { width:92%; height:14px; margin:5px 0 9px; }
:global(.sk-line-short) { width:65%; }
:global(.sk-button) { width:72px; height:24px; }
:global(.sk-pagination) { width:180px; height:32px; }
@media(max-width:768px) { :global(.sk-pagination) { height:24px; } }
:global(.sk-icon) { width:48px; height:48px; flex-shrink:0; border-radius:8px; }
@media(prefers-reduced-motion:reduce) { :global(.skeleton-shell::after) { animation:none; display:none; } }
</style>
