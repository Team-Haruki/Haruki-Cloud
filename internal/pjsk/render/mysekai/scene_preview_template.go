package mysekai

import (
	_ "embed"
	"strings"
)

//go:embed scene_preview/template.html
var scenePreviewTemplateHTML string

const scenePreviewBootstrapNeedle = `    tick();
    loadOptionsFromStorage();
    applyHudVisibility();
    applyDebugVisibility();
    reload();`

const scenePreviewBootstrapReplacement = `    function scenePreviewExportParams() {
      return new URLSearchParams(window.location.search || "");
    }

    function scenePreviewParseSites(params) {
      const raw = String(params.get("sites") || "1,2,3,4");
      const out = [];
      const seen = new Set();
      for (const part of raw.split(/[,\s]+/)) {
        const n = Number(part);
        if (!Number.isFinite(n) || n < 1 || n > 4 || seen.has(n)) continue;
        seen.add(n);
        out.push(n);
      }
      return out.length ? out : [1, 2, 3, 4];
    }

    function scenePreviewSiteLabel(siteId) {
      const n = Number(siteId);
      return n <= 1 ? "室外" : String(n - 1) + "F";
    }

    function scenePreviewSiteExists(layout, siteId) {
      const sites = layout?.userMysekaiSiteHousingLayouts;
      if (!Array.isArray(sites)) return true;
      return sites.some((site) => Number(site?.mysekaiSiteId) === Number(siteId));
    }

    function scenePreviewRenderableSites(layout, sites) {
      const rank = Number(layout?.mysekaiRank || 1);
      const out = sites.filter((siteId) => scenePreviewSiteExists(layout, siteId) && !!getSiteLevelIdByRank(rank, siteId));
      return out.length ? out : sites;
    }

    const originalGetSiteLevelIdByRank = getSiteLevelIdByRank;
    getSiteLevelIdByRank = function(mysekaiRank, siteId) {
      if (!rankReleases.length || !siteLevels.length || !siteLayouts.length) return -Number(siteId || 1);
      return originalGetSiteLevelIdByRank(mysekaiRank, siteId);
    };

    const originalGetSiteSize = getSiteSize;
    getSiteSize = function(siteLevelId) {
      if (Number(siteLevelId) < 0) return { width: 80, depth: 80, height: 10 };
      return originalGetSiteSize(siteLevelId);
    };

    function scenePreviewFitFrontCamera(tileWidth, tileHeight) {
      scene.updateMatrixWorld(true);
      const box = new THREE.Box3();
      const include = (obj) => {
        if (!obj || obj.visible === false) return;
        obj.updateMatrixWorld(true);
        const objBox = new THREE.Box3().setFromObject(obj);
        if (!objBox.isEmpty()) box.union(objBox);
      };
      include(grass);
      include(contentGroup);
      for (const wall of indoorWallPlanes) include(wall);
      if (box.isEmpty()) box.setFromCenterAndSize(new THREE.Vector3(0, 5, 0), new THREE.Vector3(80, 12, 80));

      const size = new THREE.Vector3();
      const center = new THREE.Vector3();
      box.getSize(size);
      box.getCenter(center);
      const paddedWidth = Math.max(24, size.x * 1.14);
      const paddedHeight = Math.max(12, Math.max(size.y * 1.15, size.z * 0.42));
      camera.aspect = tileWidth / tileHeight;
      camera.fov = 36;
      camera.near = 0.1;
      camera.far = 3000;
      camera.updateProjectionMatrix();

      const fov = THREE.MathUtils.degToRad(camera.fov);
      const distanceForHeight = (paddedHeight * 0.5) / Math.tan(fov * 0.5);
      const distanceForWidth = ((paddedWidth / camera.aspect) * 0.5) / Math.tan(fov * 0.5);
      const distance = Math.max(distanceForHeight, distanceForWidth, 12);
      const targetY = Math.max(box.min.y + size.y * 0.22, center.y - Math.max(0, size.y * 0.14));
      const cameraLift = Math.max(10, Math.min(54, size.z * 0.36 + size.y * 0.28));
      camera.position.set(center.x, targetY + cameraLift, box.max.z + distance);
      camera.up.set(0, 1, 0);
      controls.target.set(center.x, targetY, center.z);
      controls.update();
      markBackFacingOpacityDirty();
      applyBackFacingOpacity();
    }

    async function scenePreviewRenderOne(layoutPath, siteId, tileWidth, tileHeight) {
      siteIdEl.value = String(siteId);
      renderer.setSize(tileWidth, tileHeight);
      renderer.setPixelRatio(1);
      camera.aspect = tileWidth / tileHeight;
      camera.updateProjectionMatrix();
      await buildScene(layoutPath, getAlwaysEnabledLayoutTypes(), siteId, { forceFreshLayout: true });
      scenePreviewFitFrontCamera(tileWidth, tileHeight);
      for (let i = 0; i < 3; i++) {
        controls.update();
        applyBackFacingOpacity();
        renderer.render(scene, camera);
        await new Promise((resolve) => requestAnimationFrame(resolve));
      }
      renderer.render(scene, camera);
      return renderer.domElement;
    }

    async function runHeadlessScenePreviewExport() {
      const params = scenePreviewExportParams();
      window.__MYSEKAI_PREVIEW_READY = false;
      window.__MYSEKAI_PREVIEW_ERROR = "";
      document.documentElement.dataset.mysekaiPreviewReady = "false";
      document.documentElement.dataset.mysekaiPreviewError = "";
      try {
        const tileWidth = Math.max(640, Math.min(3200, Number(params.get("tileWidth") || 1600)));
        const tileHeight = Math.max(360, Math.min(2400, Number(params.get("tileHeight") || 900)));
        const sites = scenePreviewParseSites(params);
        const layoutPath = params.get("layout") || "/layout.json";
        const backOpacity = Math.max(0, Math.min(100, Number(params.get("backWallOpacity") || 20)));

	    app.style.width = String(tileWidth) + "px";
	    app.style.height = String(tileHeight) + "px";
        hudVisible = false;
        gridEnabled = params.get("grid") === "1";
        shadowEnabled = params.get("shadow") !== "0";
        debugEnabled = false;
        backWallOpacityRatio = backOpacity / 100;
        hudToggleBtn.style.display = "none";
        axesView.style.display = "none";
        applyHudVisibility();
        applyDebugVisibility();

        if (!fixtureMetaMap.size) await initFixtureMeta();
        const layout = await loadJson(layoutPath, { forceFresh: true });
        const renderSites = scenePreviewRenderableSites(layout, sites);
        const finalCanvas = document.createElement("canvas");
        finalCanvas.width = tileWidth;
        finalCanvas.height = tileHeight * renderSites.length;
        const ctx2d = finalCanvas.getContext("2d");
        ctx2d.fillStyle = "#edf7ff";
        ctx2d.fillRect(0, 0, finalCanvas.width, finalCanvas.height);

        for (let idx = 0; idx < renderSites.length; idx++) {
          const siteId = renderSites[idx];
          const source = await scenePreviewRenderOne(layoutPath, siteId, tileWidth, tileHeight);
          const y = idx * tileHeight;
          ctx2d.drawImage(source, 0, y, tileWidth, tileHeight);
          ctx2d.save();
          ctx2d.fillStyle = "rgba(12, 20, 32, 0.72)";
          ctx2d.fillRect(24, y + 24, 116, 48);
          ctx2d.fillStyle = "#ffffff";
          ctx2d.font = "700 30px -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif";
          ctx2d.textBaseline = "middle";
          ctx2d.fillText(scenePreviewSiteLabel(siteId), 44, y + 49);
          ctx2d.restore();
        }

        document.body.innerHTML = "";
	    document.documentElement.style.width = String(tileWidth) + "px";
	    document.documentElement.style.height = String(finalCanvas.height) + "px";
	    document.body.style.width = String(tileWidth) + "px";
	    document.body.style.height = String(finalCanvas.height) + "px";
        document.body.style.margin = "0";
        document.body.style.overflow = "hidden";
        document.body.appendChild(finalCanvas);
        window.__MYSEKAI_PREVIEW_READY = true;
        document.documentElement.dataset.mysekaiPreviewReady = "true";
      } catch (error) {
        console.error(error);
        const message = error?.message || String(error);
        window.__MYSEKAI_PREVIEW_ERROR = message;
        document.documentElement.dataset.mysekaiPreviewError = message;
      }
    }

    if (scenePreviewExportParams().get("export") === "1") {
      runHeadlessScenePreviewExport();
    } else {
      tick();
      loadOptionsFromStorage();
      applyHudVisibility();
      applyDebugVisibility();
      reload();
    }`

func scenePreviewHTML() string {
	if strings.Contains(scenePreviewTemplateHTML, scenePreviewBootstrapReplacement) {
		return scenePreviewTemplateHTML
	}
	return strings.Replace(scenePreviewTemplateHTML, scenePreviewBootstrapNeedle, scenePreviewBootstrapReplacement, 1)
}
