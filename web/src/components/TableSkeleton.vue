<script setup lang="ts">
withDefaults(defineProps<{ columns: { label:string; width?:number; minWidth?:number }[]; rows?:number; rowHeight?:number }>(), { rows:3, rowHeight:48 })
</script>
<template><div class="skeleton-shell" aria-hidden="true"><el-table :data="Array.from({length:rows},()=>({}))" :row-style="{height:rowHeight+'px'}" style="width:100%"><el-table-column v-for="column in columns" :key="column.label" :label="column.label" :width="column.width" :min-width="column.minWidth"><template #default><span class="sk-block" style="height:14px;width:70%;margin:4px 0"></span></template></el-table-column></el-table></div></template>
