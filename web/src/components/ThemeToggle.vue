<script setup lang="ts">
import { useTheme } from '@/composables/useTheme'
const { theme, toggleTheme } = useTheme()
</script>

<template>
  <button class="theme-toggle" type="button" role="switch" :aria-checked="theme === 'dark'"
    aria-label="夜间模式" :title="theme === 'dark' ? '切换到白天模式' : '切换到夜间模式'" @click="toggleTheme">
    <span class="theme-sky" :class="{ night: theme === 'dark' }" aria-hidden="true">
      <span class="theme-stars"><i></i><i></i><i></i><i></i></span>
      <span class="theme-clouds"></span>
      <span class="theme-orb"><span class="theme-moon"><i></i><i></i><i></i></span></span>
    </span>
  </button>
</template>

<style scoped>
.theme-toggle { display:inline-flex; align-items:center; justify-content:center; flex:0 0 auto; width:72px; height:44px; padding:0; border:0; background:transparent; cursor:pointer; border-radius:24px; -webkit-tap-highlight-color:transparent; }
.theme-toggle:focus-visible { outline:2px solid var(--accent); outline-offset:3px; }
.theme-sky { display:block; position:relative; width:68px; height:34px; overflow:hidden; border-radius:20px; background:#70b4da; box-shadow:inset 0 1px 4px #17384940,0 1px 0 #ffffff40; isolation:isolate; transition:background-color var(--theme-duration) var(--theme-ease); }
.theme-sky.night { background:#202d45; }
.theme-orb { position:absolute; top:4px; left:4px; width:26px; height:26px; border-radius:50%; background:#f7d365; box-shadow:0 1px 3px #152a3e55,inset 0 1px 2px #fff9,0 0 0 9px #ffffff18,0 0 0 18px #ffffff14,0 0 0 27px #ffffff0d; overflow:hidden; transform:translateX(0) rotate(0); transition:transform var(--theme-duration) var(--theme-ease); }
.night .theme-orb { transform:translateX(34px) rotate(12deg); }
.theme-moon { position:absolute; inset:0; border-radius:50%; background:#d7e0ed; box-shadow:inset 0 1px 2px #fff9; transform:translateX(105%); transition:transform var(--theme-duration) var(--theme-ease); }
.night .theme-moon { transform:translateX(0); }
.theme-moon i { position:absolute; width:8px; height:8px; border-radius:50%; top:12px; left:5px; background:#9caec6; box-shadow:inset 0 1px 2px #66798c44; }
.theme-moon i:nth-child(2) { width:5px; height:5px; top:5px; left:12px; }
.theme-moon i:nth-child(3) { width:5px; height:5px; top:16px; left:17px; }
.theme-clouds { position:absolute; width:30px; height:17px; right:-4px; bottom:-5px; background:#f2f8fc; border-radius:50%; box-shadow:-12px 7px 0 3px #f2f8fc,-26px 10px 0 3px #f2f8fc,4px -10px 0 -1px #f2f8fc,-3px -5px 0 3px #d3e7f3,-19px 4px 0 4px #d3e7f3; transition:transform var(--theme-duration) var(--theme-ease),opacity var(--theme-duration) var(--theme-ease); }
.night .theme-clouds { transform:translateY(28px); opacity:0; }
.theme-stars { position:absolute; inset:0; opacity:0; transform:translateY(-18px); transition:opacity var(--theme-duration) var(--theme-ease),transform var(--theme-duration) var(--theme-ease); }
.night .theme-stars { opacity:1; transform:translateY(0); }
.theme-stars i { position:absolute; width:5px; height:5px; background:#edf5ff; clip-path:polygon(50% 0,65% 35%,100% 50%,65% 65%,50% 100%,35% 65%,0 50%,35% 35%); left:13px; top:8px; }
.theme-stars i:nth-child(2) { width:3px; height:3px; left:7px; top:21px; }
.theme-stars i:nth-child(3) { width:4px; height:4px; left:26px; top:21px; }
.theme-stars i:nth-child(4) { width:2px; height:2px; left:28px; top:8px; }
@media(prefers-reduced-motion:reduce) { .theme-toggle * { transition:none !important; } }
</style>
