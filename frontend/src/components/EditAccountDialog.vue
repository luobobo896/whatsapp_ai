<script setup>
import { ref, inject, onMounted, computed } from "vue";
import { get, put, messageForError, patch } from "../api";
import { Globe, RefreshCw } from "lucide-vue-next";
import KnowledgeBaseBindingField from "./KnowledgeBaseBindingField.vue";

const props = defineProps({ account: Object, csrfToken: String });
const emit = defineEmits(["close", "updated"]);
const showToast = inject("showToast");
const name = ref(props.account.name);
const dailyLimit = ref(props.account.dailyLimit || 30);
const replyLimit = ref(props.account.replyLimit || 30);
const kbId = ref(Array.isArray(props.account.kbId) ? [...props.account.kbId] : []);
const knowledgeBases = ref([]);
const submitting = ref(false);

// Proxy configuration
const proxyUrl = ref(props.account.proxyUrl || "");
const validatingProxy = ref(false);
const testingProxy = ref(false);
const proxyValidationMessage = ref("");

// Instance status
const instanceStatus = ref(null);
const loadingInstanceStatus = ref(false);
const restartingInstance = ref(false);

const hasProxy = computed(() => !!proxyUrl.value);
const instanceStatusText = computed(() => {
	if (!instanceStatus.value) return "未知";
	switch (instanceStatus.value.instanceStatus) {
		case "running": return "运行中";
		case "stopped": return "已停止";
		case "error": return "错误";
		case "starting": return "启动中";
		default: return instanceStatus.value.instanceStatus || "未知";
	}
});

onMounted(async () => {
  try {
    const resp = await get("/api/knowledge/bases");
    knowledgeBases.value = resp.bases || [];
  } catch (error) {
    showToast({ tone: "error", message: messageForError(error) });
  }
  // Load instance status
  loadInstanceStatus();
});

async function loadInstanceStatus() {
	loadingInstanceStatus.value = true;
	try {
		const resp = await get(`/api/accounts/${props.account.id}/instance/status`);
		instanceStatus.value = resp;
	} catch (error) {
		// Ignore error for optional status
	} finally {
		loadingInstanceStatus.value = false;
	}
}

async function testProxy() {
	if (!proxyUrl.value) {
		showToast({ tone: "info", message: "请先输入代理地址" });
		return;
	}
	testingProxy.value = true;
	try {
		const resp = await fetch(`/api/accounts/${props.account.id}/proxy/validate`, {
			method: "GET",
			headers: {
				"Content-Type": "application/json",
				"X-CSRF-Token": props.csrfToken,
			},
		});
		const data = await resp.json();
		if (data.ok) {
			showToast({ tone: "success", message: "代理连接正常" });
		} else {
			showToast({ tone: "error", message: data.message || "代理连接失败" });
		}
	} catch (error) {
		showToast({ tone: "error", message: messageForError(error) });
	} finally {
		testingProxy.value = false;
	}
}

async function saveProxy() {
	validatingProxy.value = true;
	try {
		await put(`/api/accounts/${props.account.id}/proxy`, { proxyUrl: proxyUrl.value }, props.csrfToken);
		showToast({ tone: "success", message: "代理配置已保存" });
		// Reload instance status
		loadInstanceStatus();
	} catch (error) {
		showToast({ tone: "error", message: messageForError(error) });
	} finally {
		validatingProxy.value = false;
	}
}

async function restartInstance() {
	restartingInstance.value = true;
	try {
		await fetch(`/api/accounts/${props.account.id}/instance/restart`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				"X-CSRF-Token": props.csrfToken,
			},
		});
		showToast({ tone: "success", message: "实例重启已发起" });
		// Reload status after a delay
		setTimeout(loadInstanceStatus, 2000);
	} catch (error) {
		showToast({ tone: "error", message: messageForError(error) });
	} finally {
		restartingInstance.value = false;
	}
}

async function submit() {
  submitting.value = true;
  try {
    const body = {
      name: name.value,
      dailyLimit: Number(dailyLimit.value),
      replyLimit: Number(replyLimit.value),
      kbId: kbId.value,
    };
    await patch(`/api/accounts/${props.account.id}`, body, props.csrfToken);
    showToast({ tone: "success", message: "账号已更新" });
    emit("updated");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog model-value title="编辑客服账号" width="min(680px, calc(100vw - 28px))" class="account-dialog" @close="emit('close')">
    <el-form label-position="top" class="account-form">
      <el-form-item label="账号名称">
        <el-input v-model="name" placeholder="账号名称" />
      </el-form-item>
      <el-form-item>
        <KnowledgeBaseBindingField v-model="kbId" :knowledge-bases="knowledgeBases" />
      </el-form-item>
      <div class="account-limits-grid">
        <el-form-item label="每日回复上限">
          <el-input-number v-model="dailyLimit" :min="0" :max="10000" controls-position="right" />
        </el-form-item>
        <el-form-item label="单次拉取消息数">
          <el-input-number v-model="replyLimit" :min="1" :max="500" controls-position="right" />
          <div class="account-limit-hint">每次从 WhatsApp 拉取的历史消息条数，与「每日回复上限」相互独立</div>
        </el-form-item>
      </div>

      <!-- Proxy Configuration Section -->
      <el-divider content-position="left">代理配置</el-divider>
      <el-form-item label="代理地址">
        <div class="proxy-input-group">
          <el-input
            v-model="proxyUrl"
            placeholder="http://user:pass@host:port 或 https://host:port"
            :disabled="validatingProxy"
          >
            <template #prepend>
              <Globe :size="16" />
            </template>
          </el-input>
          <el-button
            :icon="RefreshCw"
            :loading="testingProxy"
            :disabled="!proxyUrl"
            @click="testProxy"
          >
            测试
          </el-button>
        </div>
        <div class="account-limit-hint">为该客服账号指定独立的代理地址，每个客服可使用不同 IP。留空则直连。</div>
      </el-form-item>

      <!-- Instance Status Section -->
      <el-divider content-position="left">实例状态</el-divider>
      <div class="instance-status-grid">
        <div class="instance-status-item">
          <span class="instance-status-label">状态</span>
          <el-tag v-if="!loadingInstanceStatus" :type="instanceStatus?.instanceStatus === 'running' ? 'success' : 'warning'" size="small">
            {{ instanceStatusText }}
          </el-tag>
          <el-skeleton v-else :rows="1" animated />
        </div>
        <div class="instance-status-item">
          <span class="instance-status-label">端口</span>
          <span v-if="!loadingInstanceStatus">{{ instanceStatus?.gatewayPort || "-" }} / {{ instanceStatus?.gatewayWsPort || "-" }}</span>
          <el-skeleton v-else :rows="1" animated />
        </div>
        <div class="instance-status-item">
          <span class="instance-status-label">重启次数</span>
          <span v-if="!loadingInstanceStatus">{{ instanceStatus?.restartCount || 0 }}</span>
          <el-skeleton v-else :rows="1" animated />
        </div>
        <div class="instance-status-item">
          <el-button
            :icon="RefreshCw"
            :loading="restartingInstance"
            size="small"
            @click="restartInstance"
          >
            重启实例
          </el-button>
        </div>
      </div>
    </el-form>
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.account-limit-hint {
  margin: 4px 0 0;
  font-size: 11px;
  line-height: 1.45;
  color: var(--app-muted);
}

.proxy-input-group {
  display: flex;
  gap: 8px;
}

.proxy-input-group .el-input {
  flex: 1;
}

.instance-status-grid {
  display: grid;
  grid-template-columns: repeat(4, auto);
  gap: 12px;
  align-items: center;
}

.instance-status-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.instance-status-label {
  font-size: 11px;
  color: var(--app-muted);
}

.el-divider {
  margin: 16px 0;
}
</style>
