// VENDORED VERBATIM from LostCityRS/Client-TS (branch 274-GOSCAPE, commit b67894260fb06ae6162ed3a8adab506abcd7faa9)
// source: src/datastruct/Linkable2.ts  blobSHA: d3f716f25e40ed4ef47aa5aec56e4c918f6dac79
// Only this provenance header was added; file body is unmodified.
import Linkable from '#/datastruct/Linkable.js';

export default class Linkable2 extends Linkable {
    next2: Linkable2 | null = null;
    prev2: Linkable2 | null = null;

    unlink2(): void {
        if (this.prev2 !== null) {
            this.prev2.next2 = this.next2;
            if (this.next2) {
                this.next2.prev2 = this.prev2;
            }
            this.next2 = null;
            this.prev2 = null;
        }
    }
}
