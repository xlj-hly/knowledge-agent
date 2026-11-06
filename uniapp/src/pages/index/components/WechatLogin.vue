<template>
  <view class="intro">
    <text class="page-title">欢迎回来</text>
    <text class="muted">使用微信账号进入本企业，首次登录自动创建账号</text>
  </view>
  <view class="card">
    <button class="primary" :loading="busy" :disabled="busy" @click="$emit('login')">微信登录</button>
  </view>
</template>

<script>
export default {
  props: { tenant: Object, busy: Boolean },
  emits: ['login'],
}
</script>

<style scoped src="../account.css"></style>
