'use client';
import { useRef, useState } from 'react';
import { useAccess } from '@/components/auth-gate';
import { LoaderCircle, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@/components/ui/alert-dialog';

export function DeleteResourceButton({
  name,
  description,
  disabled,
  disabledReason,
  onDelete,
  onDeleted,
}: {
  name: string;
  description: string;
  disabled?: boolean;
  disabledReason?: string;
  onDelete: () => Promise<unknown>;
  onDeleted: () => void | Promise<void>;
}) {
  const { canAdmin } = useAccess();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const [error, setError] = useState('');
  const [deleted, setDeleted] = useState(false);
  async function confirm() {
    if (lock.current || deleted || !canAdmin) return;
    lock.current = true;
    setBusy(true);
    setError('');
    try {
      await onDelete();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '删除失败，请重试');
      lock.current = false;
      setBusy(false);
      return;
    }
    setOpen(false);
    setDeleted(true);
    try {
      await onDeleted();
    } catch {
      setError('删除已成功，但列表刷新失败，请刷新页面。');
      setOpen(true);
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  if (!canAdmin) return null;
  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="text-destructive"
        disabled={disabled || deleted}
        title={disabled ? disabledReason : undefined}
        aria-label={'删除' + name}
        onClick={() => {
          setError('');
          setOpen(true);
        }}
      >
        <Trash2 />
        {disabled && disabledReason ? disabledReason : '删除'}
      </Button>
      <AlertDialog
        open={open}
        onOpenChange={(next) => {
          if (!busy) setOpen(next);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除「{name}」？</AlertDialogTitle>
            <AlertDialogDescription>{description}</AlertDialogDescription>
          </AlertDialogHeader>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <AlertDialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => setOpen(false)}
            >
              {deleted ? '关闭' : '取消'}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={busy || deleted}
              onClick={() => void confirm()}
            >
              {busy && <LoaderCircle className="animate-spin" />}
              {busy ? '正在删除…' : '确认删除'}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
