import { writeFileSync } from 'node:fs';
import { makePlanSimulationProfiles } from '../../web/lib/plan-simulation-profiles.ts';
const c = (field, op, value) => ({field, op, value});
const rules = [
 {all:[{tag:'auction_demo'},{tag:'中文标签'}],none:[{tag:'excluded'}]},
 {all:[c('age','gte','18'),c('age','lte','35')],any:[{tag:'tech_interest'},{tag:'gaming_interest'}]},
 {all:[c('device','eq','android')],any:[c('device','eq','ios'),{tag:'ok'}],none:[{tag:'bad'}]},
 {all:[c('score','gte','70.25')],none:[c('score','gte','70.26')]},
 {all:[c('score','gte','0.000000001')],none:[c('score','gte','0.000000002')]},
 {all:[c('age','in','018,020,035'),c('custom','in','001,01')],none:[c('age','eq','18'),c('custom','eq','01')]},
 {any:[{tag:'tech_interest'},{tag:'gaming_interest'}]},
 {none:[c('age','gte','0')]},
 {all:[c('__proto__','eq','safe'),c('constructor','eq','literal')]},
 {none:[c('channel','in','organic,paid')],any:[c('member_level','eq','gold'),c('score','lte','12.5')]},
];
const fixtures = [];
for (const [i, rule] of rules.entries()) for (const mix of ['matched','unmatched','mixed']) {
 const result = makePlanSimulationProfiles(100,'contract-'+i,{id:'p',name:'p',slotId:'slot',activeVersion:{number:1,targeting:rule}},mix);
 fixtures.push(...result.profiles.map(profile => ({rule,profile,expected:result.context.expected[profile.userId]})));
}
if (!process.argv[2]) throw new Error('Usage: node tests/plan-sample-contract/generate.mjs <output.json>');
writeFileSync(process.argv[2],JSON.stringify(fixtures));
console.log('Generated '+fixtures.length+' contract samples');
