// One-shot generator for goscape's pkg/wordenc/encfilter/testdata/wordenc-fixtures.json.
// Run from the Engine-TS checkout (the --tsconfig-override flag resolves the
// #/ path alias from Engine-TS's tsconfig.json):
//
//   cd "$ENGINE_TS"   # path to your Engine-TS checkout
//   BUN_TMPDIR=$TMPDIR bun run --tsconfig-override tsconfig.json \
//     "$GOSCAPE"/tools/wordenc/gen-fixtures.ts \
//     2>/dev/null \
//     > "$GOSCAPE"/pkg/wordenc/encfilter/testdata/wordenc-fixtures.json
//
// Reads the wordenc cache at data/pack/client/wordenc, runs WordEnc.filter on
// each curated input, dumps {input, filtered} pairs as JSON.
//
// NOT part of any build. Re-run whenever the curated input list changes or
// the wordenc data changes upstream.

import WordEnc from '#/cache/wordenc/WordEnc.js';

WordEnc.load('data/pack');

const inputs: string[] = [
    '',
    'a',
    'hello',
    'Hello World',
    'good morning',
    'HELLO',                        // full uppercase passthrough
    'anal',                         // direct bad word
    'AnAl',                         // mixed-case bad word
    '4n4l',                         // leetspeak
    'cooks',                        // whitelist
    'visit foo.com please',         // bare URL
    'email me at foo@test.com',     // email-context domain
    '   leading spaces',
    'trailing spaces   ',
    'multiple    spaces',
    'symbols!!!!',
    'hello world',
    'no profanity here at all',
    'A B C D E F',
];

const pairs = inputs.map(input => ({
    input,
    filtered: WordEnc.filter(input),
}));

console.log(JSON.stringify(pairs, null, 2));
