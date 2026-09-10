import type { RequestTrace } from './api';

// Only durable processing timestamps prove that the exact accepted events counted.
export function receiptCounts(ids: string[], trace: RequestTrace) {
  const events = new Map(trace.events.map((event) => [event.eventId, event]));
  let processed = 0;
  let attention = 0;
  for (const id of ids) {
    const event = events.get(id);
    if (event?.processedAt) processed++;
    else if (
      event &&
      ['RECONCILE', 'DEAD_LETTERED', 'UNCONFIRMED'].includes(event.status)
    )
      attention++;
  }
  if (trace.settlement?.status === 'RECONCILE' && processed < ids.length)
    attention = Math.max(attention, 1);
  return { processed, attention, pending: ids.length - processed - attention };
}
