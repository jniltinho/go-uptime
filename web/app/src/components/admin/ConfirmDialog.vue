<!-- Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE. -->
<template>
  <!-- Fork: confirmation on top of AdminDialog. The focus starts on Cancel, and Escape and the close button cancel -->
  <AdminDialog
    :open="open"
    :title="title"
    size="md"
    :describedby="messageId"
    :initial-focus="cancelElement"
    testid="confirm-dialog"
    @close="$emit('cancel')"
  >
    <p :id="messageId" class="whitespace-pre-line text-sm text-muted-foreground dark:text-gray-400">{{ message }}</p>
    <template #footer>
      <Button ref="cancelButton" variant="outline" data-testid="confirm-cancel" @click="$emit('cancel')">Cancel</Button>
      <Button variant="destructive" data-testid="confirm-accept" @click="$emit('confirm')">{{ confirmLabel }}</Button>
    </template>
  </AdminDialog>
</template>

<script setup>
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import AdminDialog from '@/components/admin/AdminDialog.vue'
import { uniqueId } from '@/utils/dialogStack'

defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, required: true },
  message: { type: String, required: true },
  confirmLabel: { type: String, default: 'Confirm' },
})

defineEmits(['confirm', 'cancel'])

const messageId = uniqueId('confirm-dialog-message')
const cancelButton = ref(null)
const cancelElement = () => cancelButton.value
</script>
