<template>
  <view class="intro">
    <text class="page-title">{{ screen === 'register' ? '创建企业账号' : '欢迎回来' }}</text>
    <text class="muted">{{ screen === 'register' ? '注册后即可使用本企业的账号服务' : '使用企业账号登录，管理你的个人信息' }}</text>
  </view>
  <view class="card">
    <view class="tabs">
      <button class="tab" :class="{ selected: screen === 'login' }" :disabled="busy" @click="$emit('screen', 'login')">账号登录</button>
      <button class="tab" :class="{ selected: screen === 'register' }" :disabled="busy || !tenant.registration_enabled" @click="$emit('screen', 'register')">注册账号</button>
    </view>
    <view v-if="!tenant.registration_enabled" class="registration-note">本企业暂未开放注册，请联系企业管理员开通账号。</view>
    <view class="field">
      <label class="label" for="username">用户名</label>
      <input id="username" :value="draft.username" @input="update('username', $event.detail.value)" class="input" placeholder="请输入用户名" :maxlength="64" :disabled="busy" confirm-type="next" />
      <text class="hint">3–64 个英文字母、数字、下划线、点或横线</text>
    </view>
    <view v-if="screen === 'register'" class="field">
      <label class="label" for="register-nickname">昵称（可选）</label>
      <input id="register-nickname" :value="draft.nickname" @input="update('nickname', $event.detail.value)" class="input" placeholder="希望大家如何称呼你" :maxlength="-1" :disabled="busy" />
    </view>
    <view class="field">
      <label class="label" for="password">密码</label>
      <input id="password" :value="draft.password" @input="update('password', $event.detail.value)" class="input" placeholder="请输入密码" password :maxlength="-1" :disabled="busy" confirm-type="done" @confirm="$emit('submit')" />
      <text class="hint">10–72 字节，请妥善保管你的密码</text>
    </view>
    <button class="primary" :loading="busy" :disabled="busy || (screen === 'register' && !tenant.registration_enabled)" @click="$emit('submit')">{{ screen === 'register' ? '注册并登录' : '登录' }}</button>
  </view>
</template>

<script>
export default {
  props: { tenant: Object, screen: String, busy: Boolean, draft: Object },
  emits: ['screen', 'submit', 'update'],
  methods: {
    update(field, value) { this.$emit('update', { ...this.draft, [field]: value }) },
  },
}
</script>

<style scoped src="../account.css"></style>
