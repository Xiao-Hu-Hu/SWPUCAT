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
const newTextTop = ref('0')
const oldTextTop = ref('0')
const oldTextOpacity = ref(1)
const transitionEnabled = ref(false)

watch(() => props.text, async (newVal) => {
  if (newVal === displayText.value || !newVal) return

  oldText.value = displayText.value
  displayText.value = newVal
  animating.value = true
  transitionEnabled.value = false

  newTextTop.value = '100%'
  oldTextTop.value = '0'
  oldTextOpacity.value = 1

  await nextTick()
  requestAnimationFrame(() => {
    transitionEnabled.value = true
    newTextTop.value = '0'
    oldTextTop.value = '-100%'
    oldTextOpacity.value = '0'
  })

  setTimeout(() => {
    animating.value = false
    oldText.value = ''
    transitionEnabled.value = false
  }, props.duration)
})
</script>

<template>
  <span class="rolling-number">
    <span
      v-if="animating"
      class="rolling-old"
      :style="{
        opacity: oldTextOpacity,
        top: oldTextTop,
        transition: transitionEnabled ? `all ${duration}ms cubic-bezier(0.4, 0, 0.2, 1)` : 'none'
      }"
    >{{ oldText }}</span>
    <span
      class="rolling-new"
      :style="{
        top: newTextTop,
        transition: transitionEnabled ? `top ${duration}ms cubic-bezier(0.4, 0, 0.2, 1)` : 'none'
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
  height: 1em;
  line-height: 1;
}

.rolling-new,
.rolling-old {
  position: absolute;
  left: 0;
  width: 100%;
  white-space: nowrap;
}
</style>
