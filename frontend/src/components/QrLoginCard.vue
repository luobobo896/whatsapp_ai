<script setup>
import { ref, computed, inject, onUnmounted } from "vue";
import { QrCode, RefreshCw, Unplug } from "lucide-vue-next";
import { get, post, messageForError } from "../api";

const props = defineProps({ account: Object, csrfToken: String });
const emit = defineEmits(["close", "connected", "disconnected"]);
const showToast = inject("showToast");

const qrData = ref("");
const countdown = ref(0);
const qrTotal = ref(30);
const status = ref(props.account.status || "pending");
const loading = ref(false);
const errorMsg = ref("");
const connectionTimedOut = ref(false);
let timer = null;
let pollTimer = null;
let connectionTimer = null;
let connectionDeadline = 0;

onUnmounted(() => {
  clearInterval(timer);
  clearInterval(pollTimer);
  clearTimeout(connectionTimer);
});

const qrImageUrl = computed(() => qrData.value || null);

function formatTime(s) {
  const m = Math.floor(s / 60);
  const sec = s % 60;
  return `${m}:${String(sec).padStart(2, "0")}`;
}

function secondsUntil(expiresAt) {
  const expires = new Date(expiresAt).getTime();
  return Number.isFinite(expires) ? Math.max(0, Math.floor((expires - Date.now()) / 1000)) : 30;
}

async function fetchQr() {
  loading.value = true;
  errorMsg.value = "";
  connectionTimedOut.value = false;
  connectionDeadline = 0;
  clearInterval(timer);
  clearInterval(pollTimer);
  clearTimeout(connectionTimer);
  try {
    const resp = await post(`/api/accounts/${props.account.id}/qr-login`, {}, props.csrfToken);
    qrData.value = resp.qrData;
    countdown.value = secondsUntil(resp.expiresAt);
    qrTotal.value = countdown.value;
    status.value = "qr_pending";
    startCountdown();
    startPolling();
  } catch (e) {
    errorMsg.value = messageForError(e);
    showToast({ tone: "error", message: errorMsg.value });
  } finally {
    loading.value = false;
  }
}

function startCountdown() {
  clearInterval(timer);
  timer = setInterval(() => {
    countdown.value--;
    if (countdown.value <= 0) {
      clearInterval(timer);
      timer = null;
      qrData.value = "";
      status.value = "expired";
    }
  }, 1000);
}

function expireConnectionWait() {
  clearInterval(timer);
  clearInterval(pollTimer);
  clearTimeout(connectionTimer);
  timer = null;
  pollTimer = null;
  connectionTimer = null;
  qrData.value = "";
  countdown.value = 0;
  connectionTimedOut.value = true;
  status.value = "expired";
}

function startConnectionWait(expiresAt) {
  if (!connectionDeadline) {
    const serverDeadline = new Date(expiresAt).getTime();
    const maximumDeadline = Date.now() + 60000;
    connectionDeadline = Number.isFinite(serverDeadline)
      ? Math.min(serverDeadline, maximumDeadline)
      : maximumDeadline;
    connectionTimer = setTimeout(expireConnectionWait, Math.max(0, connectionDeadline - Date.now()));
  }
  if (Date.now() >= connectionDeadline) {
    expireConnectionWait();
    return;
  }
  clearInterval(timer);
  timer = null;
  qrData.value = "";
  countdown.value = 0;
  status.value = "connecting";
}

function startPolling() {
  clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    try {
      const resp = await get(`/api/accounts/${props.account.id}/qr-status`);
      if (resp.qrData && resp.qrData !== qrData.value) {
        qrData.value = resp.qrData;
        countdown.value = secondsUntil(resp.expiresAt);
        qrTotal.value = countdown.value;
        connectionTimedOut.value = false;
        status.value = "qr_pending";
        startCountdown();
      }
      if (resp.status === "connected") {
        clearInterval(timer);
        clearInterval(pollTimer);
        clearTimeout(connectionTimer);
        timer = null;
        pollTimer = null;
        connectionTimer = null;
        status.value = "connected";
        showToast({ tone: "success", message: "WhatsApp 已连接" });
        emit("connected");
      } else if (resp.status === "connecting") {
        startConnectionWait(resp.expiresAt);
      } else if (resp.status === "expired") {
        if (resp.error) {
          errorMsg.value = resp.error;
          showToast({ tone: "error", message: errorMsg.value });
        }
        if (status.value === "connecting") {
          expireConnectionWait();
        } else {
          clearInterval(timer);
          clearInterval(pollTimer);
          timer = null;
          pollTimer = null;
          qrData.value = "";
          status.value = "expired";
        }
      }
    } catch (error) {
      errorMsg.value = messageForError(error);
    }
  }, 3000);
}

async function disconnect() {
  loading.value = true;
  try {
    await post(`/api/accounts/${props.account.id}/disconnect`, {}, props.csrfToken);
    status.value = "pending";
    clearInterval(timer);
    clearInterval(pollTimer);
    clearTimeout(connectionTimer);
    timer = null;
    pollTimer = null;
    connectionTimer = null;
    connectionDeadline = 0;
    countdown.value = 0;
    showToast({ tone: "info", message: "WhatsApp 已断开连接" });
    emit("disconnected");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <el-card shadow="never" style="max-width:500px;margin:0 auto">
    <template #header>
      <div style="display:flex;align-items:center;justify-content:space-between">
        <span style="font-weight:600">{{ account.name }} — 扫码登录</span>
        <el-tag
          :type="status === 'connected' ? 'success' : status === 'expired' ? 'danger' : 'warning'"
          size="small"
        >
          {{ status === "connected" ? "已连接" : status === "connecting" ? "连接中" : status === "expired" ? "已过期" : status === "qr_pending" ? "等待扫码" : "待连接" }}
        </el-tag>
      </div>
    </template>

    <!-- Connected -->
    <div v-if="status === 'connected'" style="text-align:center;padding:24px 0">
      <div style="width:64px;height:64px;border-radius:50%;background:#e6f7ed;display:inline-flex;align-items:center;justify-content:center;margin-bottom:12px">
        <QrCode :size="32" style="color:#1fa855" />
      </div>
      <p style="font-size:16px;font-weight:600;color:#1fa855;margin:0">WhatsApp 已连接</p>
      <p style="font-size:12px;color:#6b736d;margin:8px 0 20px">客服账号已成功对接 WhatsApp</p>
      <el-button type="danger" :icon="Unplug" :loading="loading" @click="disconnect">断开连接</el-button>
    </div>

    <!-- Scanned: stop the QR countdown but keep polling for connection. -->
    <div v-else-if="status === 'connecting'" style="text-align:center;padding:24px 0">
      <div style="width:64px;height:64px;border-radius:50%;background:#e8f5f1;display:inline-flex;align-items:center;justify-content:center;margin-bottom:12px">
        <RefreshCw :size="32" class="qr-login-spinner" style="color:#128c7e" />
      </div>
      <p style="font-size:16px;font-weight:600;color:#128c7e;margin:0">正在连接 WhatsApp</p>
      <p style="font-size:12px;color:#6b736d;margin:8px 0 0">已扫码，正在确认连接状态</p>
    </div>

    <!-- QR showing: native PNG from OpenClaw + countdown -->
    <div v-else-if="qrImageUrl" style="text-align:center">
      <img
        :src="qrImageUrl"
        style="max-width:320px;height:auto;display:block;margin:0 auto"
        alt="WhatsApp QR Code"
      />
      <div style="margin:12px 0 8px">
        <el-progress
          :percentage="Math.round((countdown / qrTotal) * 100)"
          :color="countdown <= 5 ? '#d94535' : '#128c7e'"
          :stroke-width="6"
        />
      </div>
      <p style="font-size:13px;color:#6b736d;margin:0 0 8px">
        二维码有效期剩余 <strong :style="{ color: countdown <= 5 ? '#d94535' : '#128c7e' }">{{ formatTime(countdown) }}</strong>
      </p>
    </div>

    <!-- Expired -->
    <div v-else-if="status === 'expired'" style="text-align:center;padding:24px 0">
      <p style="color:#d94535;margin-bottom:16px">{{ connectionTimedOut ? "连接超时，请重新获取二维码" : "二维码已过期，请重新获取" }}</p>
      <el-button type="primary" :icon="RefreshCw" :loading="loading" @click="fetchQr">重新获取二维码</el-button>
    </div>

    <!-- Initial state -->
    <div v-else style="text-align:center;padding:24px 0">
      <div style="width:64px;height:64px;border-radius:50%;background:#f5f7fa;display:inline-flex;align-items:center;justify-content:center;margin-bottom:12px">
        <QrCode :size="32" style="color:#6b736d" />
      </div>
      <p style="color:#6b736d;margin-bottom:16px">点击下方按钮获取 WhatsApp 登录二维码</p>
      <el-button type="primary" :icon="QrCode" :loading="loading" @click="fetchQr">扫码登录</el-button>
    </div>

    <div v-if="errorMsg" style="margin-top:8px;text-align:center">
      <span style="color:#d94535;font-size:12px">{{ errorMsg }}</span>
    </div>
  </el-card>
</template>

<style scoped>
.qr-login-spinner {
  animation: qr-login-spin 1s linear infinite;
}

@keyframes qr-login-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
