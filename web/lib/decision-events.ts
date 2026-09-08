import type { Decision } from './api';

export type DecisionEventType = 'impression' | 'click' | 'conversion';
export type DecisionEventStatus = 'ready' | 'pending' | 'recorded';
export type DecisionEventState = Record<DecisionEventType, DecisionEventStatus>;

export interface DecisionEventSubmission {
  eventId: string;
  requestId: string;
  campaignId: string;
  creativeId: string;
  type: DecisionEventType;
  valueFen?: number;
}

const emptyState = (): DecisionEventState => ({
  impression: 'ready',
  click: 'ready',
  conversion: 'ready',
});

export function canRecordDecisionEvent(
  state: DecisionEventState,
  type: DecisionEventType,
) {
  return (
    state[type] === 'ready' &&
    !Object.values(state).includes('pending') &&
    (type === 'impression' || state.impression === 'recorded')
  );
}

// This controller claims a submission synchronously, before React can render
// disabled buttons. A failed/uncertain response retains its ID for safe retries.
export function createDecisionEventController(
  createID: (type: DecisionEventType) => string,
) {
  let decision: Decision | null = null;
  let state = emptyState();
  let eventIDs: Partial<Record<DecisionEventType, string>> = {};
  let pending: DecisionEventSubmission | null = null;

  return {
    select(next: Decision | null) {
      decision = next?.matched ? { ...next } : null;
      state = emptyState();
      eventIDs = {};
      pending = null;
    },
    snapshot(): DecisionEventState {
      return { ...state };
    },
    isPending() {
      return pending !== null;
    },
    begin(type: DecisionEventType): DecisionEventSubmission | null {
      if (!decision || !canRecordDecisionEvent(state, type)) return null;
      const eventId = eventIDs[type] ?? (eventIDs[type] = createID(type));
      pending = {
        eventId,
        requestId: decision.requestId,
        campaignId: decision.campaignId,
        creativeId: decision.creativeId,
        type,
        ...(type === 'conversion' ? { valueFen: 500 } : {}),
      };
      state = { ...state, [type]: 'pending' };
      return pending;
    },
    finish(submission: DecisionEventSubmission, succeeded: boolean) {
      // A completion from a previous decision (or an earlier retry) cannot
      // unlock or mark an event belonging to the current decision.
      if (pending !== submission) return false;
      state = {
        ...state,
        [submission.type]: succeeded ? 'recorded' : 'ready',
      };
      pending = null;
      return true;
    },
  };
}
