<script setup>
import { ref } from 'vue'

const title = ref('匿名文件仓库')
fetch('/api/info').then(r => r.json()).then(d => { if (d.site_title) title.value = d.site_title }).catch(() => {})
</script>

<template>
  <div class="wrap">
    <header class="topbar">
      <div class="brand"><span class="dot">●</span> {{ title }}</div>
      <nav class="nav">
        <router-link to="/">首页</router-link>
        <router-link to="/admin">管理</router-link>
      </nav>
    </header>
    <router-view />
  </div>
</template>
