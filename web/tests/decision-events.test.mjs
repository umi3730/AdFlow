import assert from 'node:assert/strict';
import test from 'node:test';
import {
  canRecordDecisionEvent,
  createDecisionEventController,
} from '../lib/decision-events.ts';

const decision = {
  requestId: 'request-1',
  campaignId: 'campaign-1',
  creativeId: 'creative-1',
  matched: true,
  reason: 'matched',
  reservationToken: 'reservation-1',
};

function setup() {
  const IDs = [];
  const controller = createDecisionEventController((type) => {
    const id = `${type}-${IDs.length + 1}`;
    IDs.push(id);
    return id;
  });
  controller.select(decision);
  return { controller, IDs };
}

test('double clicks are synchronously rejected before any render or await', () => {
  const { controller, IDs } = setup();
  const first = controller.begin('impression');
  assert.equal(controller.begin('impression'), null);
  assert.equal(controller.begin('click'), null);
  assert.equal(controller.isPending(), true);
  assert.deepEqual(IDs, ['impression-1']);
  assert.equal(controller.finish(first, true), true);
  assert.equal(controller.snapshot().impression, 'recorded');
  assert.equal(controller.begin('impression'), null);
});

test('click and conversion require successful exposure, including after failure', () => {
  const { controller } = setup();
  for (const type of ['click', 'conversion']) {
    assert.equal(controller.begin(type), null);
    assert.equal(canRecordDecisionEvent(controller.snapshot(), type), false);
  }
  const exposure = controller.begin('impression');
  assert.equal(controller.begin('conversion'), null);
  controller.finish(exposure, false);
  assert.equal(controller.begin('click'), null);
  controller.finish(controller.begin('impression'), true);
  assert.equal(canRecordDecisionEvent(controller.snapshot(), 'click'), true);
  assert.equal(
    canRecordDecisionEvent(controller.snapshot(), 'conversion'),
    true,
  );
});

test('failed or uncertain responses retry the identical event ID and payload', () => {
  const { controller, IDs } = setup();
  const first = controller.begin('impression');
  controller.finish(first, false);
  const retry = controller.begin('impression');
  assert.deepEqual(retry, first);
  assert.notEqual(retry, first);
  assert.deepEqual(IDs, ['impression-1']);
  assert.equal(controller.finish(first, true), false);
  assert.equal(controller.snapshot().impression, 'pending');
  controller.finish(retry, true);

  const conversion = controller.begin('conversion');
  assert.equal(conversion.valueFen, 500);
  controller.finish(conversion, false);
  assert.deepEqual(controller.begin('conversion'), conversion);
  assert.deepEqual(IDs, ['impression-1', 'conversion-2']);
});

test('different event types cannot run concurrently and successful events stay complete', () => {
  const { controller } = setup();
  controller.finish(controller.begin('impression'), true);
  const click = controller.begin('click');
  assert.equal(controller.begin('conversion'), null);
  controller.finish(click, true);
  const conversion = controller.begin('conversion');
  assert.notEqual(conversion.eventId, click.eventId);
  controller.finish(conversion, true);
  assert.deepEqual(controller.snapshot(), {
    impression: 'recorded',
    click: 'recorded',
    conversion: 'recorded',
  });
  for (const type of ['impression', 'click', 'conversion']) {
    assert.equal(controller.begin(type), null);
  }
});

test('switching decisions isolates IDs and ignores a previous in-flight completion', () => {
  const { controller } = setup();
  const previous = controller.begin('impression');
  controller.select({
    ...decision,
    requestId: 'request-2',
    campaignId: 'campaign-2',
    creativeId: 'creative-2',
  });
  assert.equal(controller.begin('click'), null);
  const current = controller.begin('impression');
  assert.notEqual(current.eventId, previous.eventId);
  assert.equal(current.requestId, 'request-2');
  assert.equal(current.campaignId, 'campaign-2');
  assert.equal(current.creativeId, 'creative-2');
  assert.equal(controller.finish(previous, true), false);
  assert.equal(controller.snapshot().impression, 'pending');
  assert.equal(controller.finish(current, true), true);
});

test('clearing a user/slot or selecting an unmatched decision blocks stale callbacks', () => {
  const { controller } = setup();
  const previous = controller.begin('impression');
  controller.select(null);
  assert.equal(controller.finish(previous, true), false);
  assert.equal(controller.begin('impression'), null);
  controller.select({ ...decision, matched: false });
  assert.equal(controller.begin('impression'), null);
  controller.select(decision);
  assert.equal(controller.snapshot().impression, 'ready');
  assert.equal(controller.begin('click'), null);
});

test('view snapshots cannot mutate submission state', () => {
  const { controller } = setup();
  controller.snapshot().impression = 'recorded';
  assert.equal(controller.begin('click'), null);
});
