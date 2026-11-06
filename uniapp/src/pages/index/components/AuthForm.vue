<template>
  <view class="intro">
    <text class="page-title">欢迎回来</text>
    <text class="muted">使用企业账号登录，管理你的个人信息</text>
  </view>
  <view class="card">
    <view class="field">
      <label class="label" for="username">用户名</label>
      <input id="username" :value="draft.username" @input="update('username', $event.detail.value)" class="input" placeholder="请输入用户名" :maxlength="64" :disabled="busy" confirm-type="next" />
      <text class="hint">3–64 个英文字母、数字、下划线、点或横线</text>
    </view>
    <view class="field">
      <label class="label" for="password">密码</label>
      <input id="password" :value="draft.password" @input="update('password', $event.detail.value)" class="input" placeholder="请输入密码" password :maxlength="-1" :disabled="busy" confirm-type="done" @confirm="$emit('submit')" />
      <text class="hint">10–72 字节，请妥善保管你的密码</text>
    </view>
    <button class="primary" :loading="busy" :disabled="busy" @click="$emit('submit')">登录</button>
  </view>
</template>

<script>
export default {
  props: { busy: Boolean, draft: Object },
  emits: ['submit', 'update'],
  methods: {
    update(field, value) { this.$emit('update', { ...this.draft, [field]: value }) },
  },
}
</script>

<style scoped src="../account.css"></style>
