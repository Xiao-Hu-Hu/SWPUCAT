import { computed, onBeforeUnmount, ref } from 'vue'

export function usePaginationLayout(fullLayout = 'total, sizes, prev, pager, next, jumper') {
  const query = window.matchMedia('(max-width: 768px)')
  const isMobile = ref(query.matches)
  const onChange = (e: MediaQueryListEvent) => {
    isMobile.value = e.matches
  }
  query.addEventListener('change', onChange)
  onBeforeUnmount(() => query.removeEventListener('change', onChange))

  const layout = computed(() => (isMobile.value ? 'total, prev, pager, next' : fullLayout))
  const pagerCount = computed(() => (isMobile.value ? 5 : 7))
  const small = computed(() => isMobile.value)

  return { layout, pagerCount, small }
}
