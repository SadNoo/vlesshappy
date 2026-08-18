<?php

namespace App\Models;

class VlessRealityNodeConfig extends Model
{
    protected $table = 'vless_reality_node_config';
    protected $primaryKey = 'node_id';
    public $incrementing = false;

    protected $casts = array(
        'node_id' => 'int',
        'public_port' => 'int',
        'config_version' => 'int',
    );
}
