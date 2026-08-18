<div class="card">
    <div class="card-main">
        <div class="card-inner">
            <h3>VLESS REALITY（仅节点类型 15 使用）</h3>
            <p class="form-control-guide"><i class="material-icons">security</i>这里只保存公开材料；REALITY 私钥必须只保存在节点 secret 文件。</p>

            <div class="form-group form-group-label">
                <label class="floating-label" for="vless_public_host">公开地址</label>
                <input class="form-control maxwidth-edit" id="vless_public_host" name="vless_public_host" type="text" value="{$vless_config->public_host|default:''}">
            </div>
            <div class="form-group form-group-label">
                <label class="floating-label" for="vless_public_port">公开端口</label>
                <input class="form-control maxwidth-edit" id="vless_public_port" name="vless_public_port" type="number" min="1" max="65535" value="{$vless_config->public_port|default:443}">
            </div>
            <div class="form-group form-group-label">
                <label class="floating-label" for="vless_server_name">REALITY SNI</label>
                <input class="form-control maxwidth-edit" id="vless_server_name" name="vless_server_name" type="text" value="{$vless_config->server_name|default:''}">
            </div>
            <div class="form-group form-group-label">
                <label class="floating-label" for="vless_target">REALITY target</label>
                <input class="form-control maxwidth-edit" id="vless_target" name="vless_target" type="text" placeholder="www.example.com:443" value="{$vless_config->target|default:''}">
            </div>
            <div class="form-group form-group-label">
                <label class="floating-label" for="vless_reality_public_key">REALITY 公钥</label>
                <input class="form-control maxwidth-edit" id="vless_reality_public_key" name="vless_reality_public_key" type="text" value="{$vless_config->reality_public_key|default:''}">
            </div>
            <div class="form-group form-group-label">
                <label class="floating-label" for="vless_short_id">REALITY short ID</label>
                <input class="form-control maxwidth-edit" id="vless_short_id" name="vless_short_id" type="text" maxlength="16" value="{$vless_config->short_id|default:''}">
            </div>
            <div class="form-group form-group-label">
                <label class="floating-label" for="vless_min_client_version">最低客户端版本（可空）</label>
                <input class="form-control maxwidth-edit" id="vless_min_client_version" name="vless_min_client_version" type="text" maxlength="32" value="{$vless_config->min_client_version|default:''}">
            </div>
        </div>
    </div>
</div>
