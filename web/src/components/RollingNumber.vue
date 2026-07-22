<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'

interface CharSlot {
  char: string
  oldChar: string
  animating: boolean
  transitioning: boolean
}

const props = withDefaults(defineProps<{
  text: string
  duration?: number
}>(), {
  duration: 450
})

const chars = ref<CharSlot[]>([])

function buildSlots(text: string): CharSlot[] {
  return text.split('').map(char => ({
    char,
    oldChar: '',
    animating: false,
    transitioning: false
  }))
}

chars.value = buildSlots(props.text)

watch(() => props.text, async (newVal) => {
  if (!newVal) return

  const oldSlots = chars.value
  const newChars = newVal.split('')

  // Length changed — rebuild all slots
  if (newChars.length !== oldSlots.length) {
    chars.value = buildSlots(newVal)
    return
  }

  // Mark changed characters
  for (let i = 0; i < newChars.length; i++) {
    if (newChars[i] !== oldSlots[i].char) {
      oldSlots[i].oldChar = oldSlots[i].char
      oldSlots[i].char = newChars[i]
      oldSlots[i].animating = true
      oldSlots[i].transitioning = false
    }
  }

  await nextTick()
  requestAnimationFrame(() => {
    for (const slot of oldSlots) {
      if (slot.animating) {
        slot.transitioning = true
      }
    }
  })

  setTimeout(() => {
    for (const slot of oldSlots) {
      if (slot.animating) {
        slot.animating = false
        slot.oldChar = ''
        slot.transitioning = false
      }
    }
  }, props.duration)
})
</script>

<template>
  <span class="rolling-number">
    <span
      v-for="(slot, i) in chars"
      :key="i"
      class="char-slot"
    >
      <span
        v-if="slot.animating"
        class="char-old"
        :class="{ move: slot.transitioning }"
        :style="{ transition: `transform ${duration}ms cubic-bezier(0.4, 0, 0.2, 1), opacity ${duration}ms cubic-bezier(0.4, 0, 0.2, 1)` }"
      >{{ slot.oldChar }}</span>
      <span
        class="char-new"
        :class="{ move: slot.animating && !slot.transitioning }"
        :style="{ transition: slot.transitioning ? `transform ${duration}ms cubic-bezier(0.4, 0, 0.2, 1)` : 'none' }"
      >{{ slot.char }}</span>
    </span>
  </span>
</template>

<style scoped>
.rolling-number {
  display: inline-flex;
  font-variant-numeric: tabular-nums;
}

.char-slot {
  position: relative;
  display: inline-block;
  overflow: hidden;
  height: 1em;
  line-height: 1;
}

.char-new {
  display: inline-block;
}

.char-new.move {
  transform: translateY(100%);
}

.char-old {
  position: absolute;
  left: 0;
  top: 0;
}

.char-old.move {
  transform: translateY(-100%);
  opacity: 0;
}
</style>
