import assert from 'node:assert/strict';
import test from 'node:test';
import {
  compileRuleDraft,
  editorFromCampaign,
  prepareAgentPlan,
} from '../lib/campaign-rules.ts';

const draft = {
  targeting: { all: [{ tag: 'auction_demo' }], any: [], none: [] },
  dailyBudgetYuan: '10.00',
  impressionCostYuan: '1.00',
  frequencyLimit: '3',
};
test('Auction publishing keeps advertiser and integer-fen bid separate from legacy cost', () => {
  const value = {
    ...draft,
    auction: {
      advertiserId: ' STUDIO-A ',
      advertiserName: ' 星河游戏 ',
      bidYuan: '0.05',
    },
  };
  const input = compileRuleDraft(value);
  assert.deepEqual(input.auction, {
    advertiserId: 'studio-a',
    advertiserName: '星河游戏',
    bidFen: 5,
  });
  assert.equal(input.impressionCostFen, 5);
  assert.equal(value.impressionCostYuan, '1.00');
  const old = compileRuleDraft(draft);
  assert.equal(old.impressionCostFen, 100);
  assert.equal('auction' in old, false);
  const edited = editorFromCampaign({ activeVersion: { number: 1, ...input } });
  assert.equal(edited.auction.bidYuan, '0.05');
  assert.equal(prepareAgentPlan(edited).auction.advertiserId, 'studio-a');
});
test('Auction configuration rejects invalid owner, precision and unaffordable bids', () => {
  const valid = {
    advertiserId: 'studio-a',
    advertiserName: '星河游戏',
    bidYuan: '0.05',
  };
  for (const change of [
    { advertiserId: 'bad id' },
    { advertiserName: 'a' },
    { bidYuan: '0' },
    { bidYuan: '0.001' },
    { bidYuan: '11' },
    { bidYuan: 'NaN' },
  ])
    assert.throws(() =>
      compileRuleDraft({ ...draft, auction: { ...valid, ...change } }),
    );
});
