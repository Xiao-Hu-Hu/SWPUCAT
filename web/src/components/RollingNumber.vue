<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'

const props = withDefaults(defineProps<{
  text: string
  duration?: number
}>(), {
  duration: 500
})

const displayText = ref(props.text)
const oldText = ref('')
const animating = ref(false)
const newTransform = ref('translateY(0)')
const oldTransform = ref('translateY(0)')
const oldOpacity = ref(1)
const useTransition = ref(false)

watch(() => props.text, async (newVal) => {
  if (newVal === displayText.value || !newVal) return

  oldText.value = displayText.value
  displayText.value = newVal
  animating.value = true
  useTransition.value = false

  // Position new text below, old text at origin
  newTransform.value = 'translateY(100%)'
  oldTransform.value = 'translateY(0)'
  oldOpacity.value = 1

  await nextTick()
  requestAnimationFrame(() => {
    useTransition.value = true
    newTransform.value = 'translateY(0)'
    oldTransform.value = 'translateY(-100%)'
    oldOpacity.value = '0'
  })

  setTimeout(() => {
    animating.value = false
    oldText.value = ''
    useTransition.value = false
  }, props.duration)
})
</script>

<template>
  <span class="rolling-number">
    <span
      v-if="animating"
      class="rolling-old"
      :style="{
        opacity: oldOpacity,
        transform: oldTransform,
        transition: useTransition ? `transform ${duration}ms cubic-bezier(0.4, 0, 0.2, 1), opacity ${duration}ms cubic-bezier(0.4, 0, 0.2, 1)` : 'none'
      }"
    >{{ oldText }}</span>
    <span
      class="rolling-new"
      :style="{
        transform: newTransform,
        transition: useTransition ? `transform ${duration}ms cubic-bezier(0.4, 0, 0.2, 1)` : 'none'
      }"
    >{{ displayText }}</span>
  </span>
</template>

<style scoped>
.rolling-number {
  position: relative;
  display: inline-block;
  overflow: hidden;
  vertical-align: baseline;
}

.rolling-new {
  display: block;
}

.rolling-old {
  position: absolute;
  left: 0;
  top: 0;
  width: 100%;
  white-space: nowrap;
}
</style>
