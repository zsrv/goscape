// VENDORED VERBATIM from LostCityRS/Client-TS (branch 274-GOSCAPE, commit b67894260fb06ae6162ed3a8adab506abcd7faa9)
// source: src/dash3d/ModelSource.ts  blobSHA: d778ab5aa3eb853184952be38b4cff4597f8da4e
// Only this provenance header was added; file body is unmodified.
import Linkable2 from '#/datastruct/Linkable2.js';
import type PointNormal from '#/dash3d/PointNormal.js';
import type Model from '#/dash3d/Model.js';

export default class ModelSource extends Linkable2 {
    public pointNormal: (PointNormal | null)[] | null = null;
    public minY: number = 1000;

    worldRender(yaw: number, sinEyePitch: number, cosEyePitch: number, sinEyeYaw: number, cosEyeYaw: number, relativeX: number, relativeY: number, relativeZ: number, typecode: number): void {
        const model = this.getTempModel();
        if (model) {
            this.minY = model.minY;
            model.worldRender(yaw, sinEyePitch, cosEyePitch, sinEyeYaw, cosEyeYaw, relativeX, relativeY, relativeZ, typecode);
        }
    }

    getTempModel(): Model | null {
        return null;
    }
}
