// VENDORED VERBATIM from LostCityRS/Client-TS (branch 274-GOSCAPE, commit b67894260fb06ae6162ed3a8adab506abcd7faa9)
// source: src/dash3d/AnimBase.ts  blobSHA: 136e18b7f296afc6b606bdf56e33038cdafdc068
// Only this provenance header was added; file body is unmodified.
import Packet from '#/io/Packet.js';

import { TypedArray1d } from '#/util/Arrays.js';

export const enum AnimTransform {
    ORIGIN = 0,
    TRANSLATE = 1,
    ROTATE = 2,
    SCALE = 3,
    TRANSPARENCY = 5
}

export default class AnimBase {
    size: number = 0;
    type: Uint8Array | null = null;
    labels: (Uint8Array | null)[] | null = null;

    constructor(buf: Packet) {
        this.size = buf.g1();

        this.type = new Uint8Array(this.size);
        this.labels = new TypedArray1d(this.size, null);

        for (let i = 0; i < this.size; i++) {
            this.type[i] = buf.g1();
        }

        for (let i = 0; i < this.size; i++) {
            const count = buf.g1();
            this.labels[i] = new Uint8Array(count);

            for (let j = 0; j < count; j++) {
                this.labels[i]![j] = buf.g1();
            }
        }
    }
}
