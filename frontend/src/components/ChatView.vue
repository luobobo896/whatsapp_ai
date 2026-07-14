<script setup>
import { ref, watch, nextTick, onUnmounted } from "vue";
import { ArrowLeft, SendHorizontal } from "lucide-vue-next";
import { get, post, messageForError } from "../api";
import { formatDate } from "../utils";

const props = defineProps({ account: Object, csrfToken: String });
const emit = defineEmits(["back"]);

const conversations = ref([]);
const selectedConv = ref(null);
const messages = ref([]);
const loadingConvs = ref(false);
const loadingMsgs = ref(false);
const replyText = ref("");
const sending = ref(false);
const chatBody = ref(null);
let pollTimer = null;

onUnmounted(() => clearInterval(pollTimer));

async function loadConversations() {
  loadingConvs.value = true;
  try {
    const resp = await get(`/api/conversations?accountId=${props.account.id}`);
    conversations.value = resp.conversations || [];
  } catch (e) {
    /* ignore */
  } finally {
    loadingConvs.value = false;
  }
}

async function loadMessages(conv) {
  selectedConv.value = conv;
  loadingMsgs.value = true;
  try {
    const limit = props.account.replyLimit || 30;
    const resp = await get(`/api/conversations/${conv.conversationId}/messages?limit=${limit}`);
    messages.value = (resp.messages || []).reverse();
  } catch (e) {
    messages.value = [];
  } finally {
    loadingMsgs.value = false;
    await nextTick();
    scrollToBottom();
  }
}

function scrollToBottom() {
  if (chatBody.value) {
    chatBody.value.scrollTop = chatBody.value.scrollHeight;
  }
}

async function sendReply() {
  const text = replyText.value.trim();
  if (!text || !selectedConv.value) return;
  sending.value = true;
  try {
    await post(
      "/api/conversations/messages",
      {
        conversationId: selectedConv.value.conversationId,
        accountId: props.account.id,
        customerName: selectedConv.value.customerName,
        role: "agent",
        content: text,
        knowledgeIds: "[]",
      },
      props.csrfToken,
    );
    replyText.value = "";
    // Reload messages
    await loadMessages(selectedConv.value);
  } catch (e) {
    /* ignore */
  } finally {
    sending.value = false;
  }
}

function handleKeydown(e) {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    sendReply();
  }
}

// Poll for new messages
function startPolling(conv) {
  clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    try {
      const limit = props.account.replyLimit || 30;
      const resp = await get(`/api/conversations/${conv.conversationId}/messages?limit=${limit}`);
      const latest = (resp.messages || []).reverse();
      if (latest.length !== messages.value.length) {
        messages.value = latest;
        await nextTick();
        scrollToBottom();
      }
    } catch { /* ignore */ }
  }, 5000);
}

watch(selectedConv, (conv) => {
  if (conv) {
    startPolling(conv);
  } else {
    clearInterval(pollTimer);
  }
});

loadConversations();
</script>

<template>
  <div style="display:flex;height:calc(100vh - 120px);gap:0;border:1px solid #e2e6e3;border-radius:8px;overflow:hidden;background:#fff">
    <!-- Conversation list sidebar -->
    <div style="width:280px;flex-shrink:0;border-right:1px solid #e2e6e3;display:flex;flex-direction:column">
      <div style="padding:12px 14px;border-bottom:1px solid #e2e6e3;display:flex;align-items:center;gap:8px">
        <el-button text :icon="ArrowLeft" @click="emit('back')" />
        <span style="font-weight:600;font-size:14px">{{ account.name }}</span>
      </div>
      <div style="flex:1;overflow-y:auto" v-loading="loadingConvs">
        <div
          v-for="conv in conversations"
          :key="conv.conversationId"
          @click="loadMessages(conv)"
          :style="{
            padding: '12px 14px',
            cursor: 'pointer',
            borderBottom: '1px solid #f0f2f5',
            background: selectedConv?.conversationId === conv.conversationId ? '#e1f5f0' : 'transparent',
          }"
        >
          <div style="display:flex;justify-content:space-between;align-items:center">
            <span style="font-weight:600;font-size:13px">{{ conv.customerName }}</span>
            <span style="font-size:10px;color:#949e96">{{ formatDate(conv.lastMessageAt) }}</span>
          </div>
          <div style="font-size:12px;color:#6b736d;margin-top:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">
            {{ conv.lastMessage }}
          </div>
          <el-tag size="small" type="info" style="margin-top:4px">{{ conv.messageCount }} 条</el-tag>
        </div>
        <div v-if="!conversations.length && !loadingConvs" style="text-align:center;color:#6b736d;padding:40px 20px;font-size:13px">
          暂无会话
        </div>
      </div>
    </div>

    <!-- Chat area -->
    <div style="flex:1;display:flex;flex-direction:column;min-width:0">
      <!-- Chat body -->
      <div ref="chatBody" style="flex:1;overflow-y:auto;padding:16px;background:#efeae2">
        <div v-if="!selectedConv" style="display:flex;align-items:center;justify-content:center;height:100%;color:#6b736d;font-size:14px">
          选择一个会话开始查看
        </div>
        <div v-else-if="loadingMsgs" style="display:flex;align-items:center;justify-content:center;height:100%">
          <el-icon class="is-loading" :size="24" />
        </div>
        <div v-else>
          <div v-if="!messages.length" style="text-align:center;color:#6b736d;padding:40px;font-size:13px">
            暂无消息
          </div>
          <div
            v-for="m in messages"
            :key="m.id"
            :style="{
              display: 'flex',
              justifyContent: m.role === 'customer' ? 'flex-start' : 'flex-end',
              marginBottom: '10px',
            }"
          >
            <div
              :style="{
                maxWidth: '70%',
                padding: '8px 14px',
                borderRadius: m.role === 'customer' ? '0 12px 12px 12px' : '12px 0 12px 12px',
                background: m.role === 'customer' ? '#fff' : '#d9fdd3',
                boxShadow: '0 1px 2px rgba(0,0,0,.08)',
              }"
            >
              <div style="font-size:11px;font-weight:700;color:#128c7e;margin-bottom:3px">
                {{ m.role === "customer" ? m.customerName || "客户" : "客服" }}
              </div>
              <div style="font-size:13px;line-height:1.5;white-space:pre-wrap;word-break:break-word">
                {{ m.content }}
              </div>
              <div style="font-size:10px;color:#949e96;margin-top:4px;text-align:right">
                {{ formatDate(m.createdAt) }}
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Input area -->
      <div
        v-if="selectedConv"
        style="padding:12px 16px;border-top:1px solid #e2e6e3;background:#f5f7fa;display:flex;gap:10px;align-items:flex-end"
      >
        <el-input
          v-model="replyText"
          type="textarea"
          :rows="2"
          placeholder="输入回复内容... (Enter 发送)"
          @keydown="handleKeydown"
          style="flex:1"
        />
        <el-button type="primary" :icon="SendHorizontal" :loading="sending" :disabled="!replyText.trim()" @click="sendReply">
          发送
        </el-button>
      </div>
    </div>
  </div>
</template>
