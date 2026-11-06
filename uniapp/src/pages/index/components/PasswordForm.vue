<template>
  <view class="intro"><text class="page-title">修改登录密码</text><text class="muted">修改后所有设备将退出登录，请重新登录</text></view>
  <view class="card">
    <view v-if="!wechat" class="field">
      <label class="label" for="current-password">当前密码</label>
      <input id="current-password" :value="draft.current_password" @input="update('current_password', $event.detail.value)" class="input" placeholder="请输入当前密码" password :maxlength="-1" :disabled="busy" />
    </view>
    <view class="field">
      <label class="label" for="new-password">新密码</label>
      <input id="new-password" :value="draft.new_password" @input="update('new_password', $event.detail.value)" class="input" placeholder="请输入新密码" password :maxlength="-1" :disabled="busy" />
      <text class="hint">10–72 字节，中文等字符占用多个字节</text>
    </view>
    <view class="field">
      <label class="label" for="confirm-password">确认新密码</label>
      <input id="confirm-password" :value="draft.confirmation" @input="update('confirmation', $event.detail.value)" class="input" placeholder="请再次输入新密码" password :maxlength="-1" :disabled="busy" />
    </view>
    <button class="primary" :loading="busy" :disabled="busy" @click="$emit('submit')">确认修改密码</button>
    <button class="text-button" :disabled="busy" @click="$emit('back')">返回我的账号</button>
  </view>
</template>

<script>
export default {
  props: { busy: Boolean, draft: Object, wechat: Boolean },
  emits: ['back', 'submit', 'update'],
  methods: {
    update(field, value) { this.$emit('update', { ...this.draft, [field]: value }) },
  },
}
</script>

<style scoped src="../account.css"></style>
