export function shapeDevices(resp: any, limit = 500): any {
  return {
    cache_age_ms: resp.cache_age_ms,
    devices: (resp.devices ?? []).slice(0, limit).map((d: any) => ({
      device_id: d.device_id,
      platform: d.platform,
      name: d.name,
      model: d.model,
      os_version: d.os_version,
      is_simulator: d.is_simulator,
      status: d.status
    }))
  };
}

export function shapeTree(resp: any, nodeLimit: number): any {
  const nodes = (resp.nodes ?? []).slice(0, nodeLimit).map((n: any) => ({
    ref_id: n.ref_id,
    parent_ref_id: n.parent_ref_id,
    index: n.index,
    text: n.text,
    content_desc: n.content_desc,
    resource_id: n.resource_id,
    class_name: n.class_name,
    package_name: n.package_name,
    bounds: n.bounds,
    enabled: n.enabled,
    clickable: n.clickable,
    visible: n.visible
  }));

  return {
    device_id: resp.device_id,
    snapshot_id: resp.snapshot_id,
    expires_at_unix_ms: resp.expires_at_unix_ms,
    total_nodes: resp.total_nodes,
    next_cursor: resp.next_cursor,
    nodes
  };
}

export function shapeElements(resp: any, includeNodes: boolean, limit: number): any {
  const elements = (resp.elements ?? []).slice(0, limit).map((el: any) => {
    if (!includeNodes) {
      return { ref_id: el.ref_id };
    }
    return {
      ref_id: el.ref_id,
      node: {
        text: el.node?.text,
        content_desc: el.node?.content_desc,
        resource_id: el.node?.resource_id,
        class_name: el.node?.class_name,
        bounds: el.node?.bounds,
        clickable: el.node?.clickable,
        visible: el.node?.visible
      }
    };
  });

  return {
    device_id: resp.device_id,
    snapshot_id: resp.snapshot_id,
    total_matched: resp.total_matched,
    next_cursor: resp.next_cursor,
    elements
  };
}

export function shapeAction(resp: any): any {
  return {
    device_id: resp.device_id,
    action_id: resp.action_id,
    status: resp.status,
    started_at_unix_ms: resp.started_at_unix_ms,
    completed_at_unix_ms: resp.completed_at_unix_ms,
    error_code: resp.error_code,
    error_message: resp.error_message,
    metadata: resp.metadata
  };
}

export function shapeScreenshotEvents(events: any[]): any {
  const frames: any[] = [];
  let totalChunks = 0;
  for (const event of events) {
    if (event.frame_meta) {
      frames.push(event.frame_meta);
    }
    if (event.chunk) {
      totalChunks += 1;
    }
  }
  return {
    frame_count: frames.length,
    chunk_count: totalChunks,
    frames
  };
}
