<?php

namespace App\Services;

use App\Models\Node;
use App\Models\User;
use InvalidArgumentException;

class VlessReality
{
    public const NODE_SORT = 15;

    public static function nodesForUser(User $user)
    {
        $query = Node::where('sort', self::NODE_SORT)
            ->where('type', 1)
            ->where(static function ($bandwidth) {
                $bandwidth->where('node_bandwidth_limit', 0)
                    ->orWhereRaw('node_bandwidth < node_bandwidth_limit');
            });

        if (!$user->is_admin) {
            $query->where('node_class', '<=', $user->class)
                ->where(static function ($group) use ($user) {
                    $group->where('node_group', 0)
                        ->orWhere('node_group', $user->node_group);
                });
        }

        return $query->orderBy('name')->orderBy('id')->get();
    }

    public static function link(User $user, Node $node)
    {
        $config = self::configForNode($node);
        $host = strpos($config->public_host, ':') !== false
            ? '[' . $config->public_host . ']'
            : $config->public_host;
        $query = http_build_query(array(
            'encryption' => 'none',
            'flow' => 'xtls-rprx-vision',
            'security' => 'reality',
            'sni' => $config->server_name,
            'fp' => 'chrome',
            'pbk' => $config->reality_public_key,
            'sid' => $config->short_id,
            'type' => 'tcp',
        ), '', '&', PHP_QUERY_RFC3986);

        return 'vless://' . rawurlencode($user->getUuid()) . '@' . $host . ':'
            . (int)$config->public_port . '?' . $query . '#'
            . rawurlencode((string)$node->name);
    }

    public static function mihomoProxy(User $user, Node $node)
    {
        $config = self::configForNode($node);
        return array(
            'name' => (string)$node->name,
            'type' => 'vless',
            'server' => (string)$config->public_host,
            'port' => (int)$config->public_port,
            'uuid' => $user->getUuid(),
            'udp' => true,
            'network' => 'tcp',
            'tls' => true,
            'servername' => (string)$config->server_name,
            'flow' => 'xtls-rprx-vision',
            'packet-encoding' => 'xudp',
            'client-fingerprint' => 'chrome',
            'reality-opts' => array(
                'public-key' => (string)$config->reality_public_key,
                'short-id' => (string)$config->short_id,
            ),
            'encryption' => '',
        );
    }

    public static function mihomoProxies(User $user)
    {
        if (!self::userIsActive($user)) {
            return array();
        }
        $proxies = array();
        foreach (self::nodesForUser($user) as $node) {
            try {
                $proxies[] = self::mihomoProxy($user, $node);
            } catch (InvalidArgumentException $exception) {
                // A malformed VLESS node must not break the mixed subscription.
            }
        }
        return $proxies;
    }

    public static function singBoxOutbound(User $user, Node $node)
    {
        $config = self::configForNode($node);
        return array(
            'type' => 'vless',
            'tag' => (string)$node->name,
            'server' => (string)$config->public_host,
            'server_port' => (int)$config->public_port,
            'uuid' => $user->getUuid(),
            'flow' => 'xtls-rprx-vision',
            'packet_encoding' => 'xudp',
            'tls' => array(
                'enabled' => true,
                'server_name' => (string)$config->server_name,
                'utls' => array(
                    'enabled' => true,
                    'fingerprint' => 'chrome',
                ),
                'reality' => array(
                    'enabled' => true,
                    'public_key' => (string)$config->reality_public_key,
                    'short_id' => (string)$config->short_id,
                ),
            ),
        );
    }

    public static function singBoxOutbounds(User $user)
    {
        if (!self::userIsActive($user)) {
            return array();
        }
        $outbounds = array();
        foreach (self::nodesForUser($user) as $node) {
            try {
                $outbounds[] = self::singBoxOutbound($user, $node);
            } catch (InvalidArgumentException $exception) {
                // Keep valid protocols when one VLESS node is malformed.
            }
        }
        return $outbounds;
    }

    public static function subscription(User $user, $nodes = null)
    {
        if (!self::userIsActive($user)) {
            return '';
        }
        if ($nodes === null) {
            $nodes = self::nodesForUser($user);
        }
        $links = array();
        foreach ($nodes as $node) {
            try {
                $links[] = self::link($user, $node);
            } catch (InvalidArgumentException $exception) {
                // A malformed admin entry must not break every subscription.
            }
        }
        return count($links) === 0 ? '' : base64_encode(implode("\n", $links) . "\n");
    }

    public static function configForNode(Node $node)
    {
        if ((int)$node->sort !== self::NODE_SORT || (int)$node->type !== 1) {
            throw new InvalidArgumentException('节点不是可见的 VLESS REALITY 节点');
        }
        return (object)self::parseServer((string)$node->server);
    }

    public static function emptyConfig()
    {
        return (object)array(
            'public_host' => '',
            'public_port' => 443,
            'server_name' => '',
            'target' => '',
            'reality_public_key' => '',
            'short_id' => '',
            'fingerprint' => 'chrome',
            'flow' => 'xtls-rprx-vision',
            'transport' => 'raw',
            'min_client_version' => '',
        );
    }

    public static function requestConfig($request)
    {
        $publicPort = trim((string)$request->getParam('vless_public_port'));
        if ($publicPort === '' || !ctype_digit($publicPort)) {
            throw new InvalidArgumentException('VLESS REALITY 公开端口必须是数字');
        }
        $config = array(
            'public_host' => trim((string)$request->getParam('vless_public_host')),
            'public_port' => (int)$publicPort,
            'server_name' => trim((string)$request->getParam('vless_server_name')),
            'target' => trim((string)$request->getParam('vless_target')),
            'reality_public_key' => trim((string)$request->getParam('vless_reality_public_key')),
            'short_id' => strtolower(trim((string)$request->getParam('vless_short_id'))),
            'fingerprint' => 'chrome',
            'flow' => 'xtls-rprx-vision',
            'transport' => 'raw',
            'min_client_version' => trim((string)$request->getParam('vless_min_client_version')),
        );
        self::validateConfig($config);
        return $config;
    }

    public static function encodeServer(array $config)
    {
        self::validateConfig($config);
        $options = array(
            'sni=' . $config['server_name'],
            'pbk=' . $config['reality_public_key'],
            'sid=' . $config['short_id'],
            'target=' . $config['target'],
        );
        if ($config['min_client_version'] !== '') {
            $options[] = 'minver=' . $config['min_client_version'];
        }
        $server = $config['public_host'] . ';' . (int)$config['public_port']
            . ';0;tcp;reality;' . implode('|', $options);
        if (strlen($server) > 255) {
            throw new InvalidArgumentException('VLESS REALITY 节点配置超过 ss_node.server 的 255 字节限制');
        }
        return $server;
    }

    public static function parseServer($server)
    {
        if ($server === '' || strlen($server) > 255) {
            throw new InvalidArgumentException('VLESS REALITY 节点配置长度无效');
        }
        $parts = explode(';', $server);
        if (count($parts) !== 6 || trim($parts[2]) !== '0'
            || strtolower(trim($parts[3])) !== 'tcp'
            || strtolower(trim($parts[4])) !== 'reality'
        ) {
            throw new InvalidArgumentException('VLESS REALITY 节点地址格式无效');
        }
        $port = trim($parts[1]);
        if ($port === '' || !ctype_digit($port)) {
            throw new InvalidArgumentException('VLESS REALITY 公开端口必须是数字');
        }
        $options = array();
        foreach (explode('|', $parts[5]) as $raw) {
            $pair = explode('=', $raw, 2);
            if (count($pair) !== 2) {
                throw new InvalidArgumentException('VLESS REALITY 节点选项格式无效');
            }
            $key = strtolower(trim($pair[0]));
            if (!in_array($key, array('sni', 'pbk', 'sid', 'target', 'minver'), true)
                || array_key_exists($key, $options)
            ) {
                throw new InvalidArgumentException('VLESS REALITY 节点包含未知或重复选项');
            }
            $options[$key] = trim($pair[1]);
        }
        foreach (array('sni', 'pbk', 'sid', 'target') as $required) {
            if (!array_key_exists($required, $options)) {
                throw new InvalidArgumentException('VLESS REALITY 节点缺少必要选项');
            }
        }
        $config = array(
            'public_host' => trim($parts[0]),
            'public_port' => (int)$port,
            'server_name' => $options['sni'],
            'target' => $options['target'],
            'reality_public_key' => $options['pbk'],
            'short_id' => strtolower($options['sid']),
            'fingerprint' => 'chrome',
            'flow' => 'xtls-rprx-vision',
            'transport' => 'raw',
            'min_client_version' => isset($options['minver']) ? $options['minver'] : '',
        );
        self::validateConfig($config);
        return $config;
    }

    public static function publicEndpoint(Node $node)
    {
        try {
            $config = self::configForNode($node);
            $host = strpos($config->public_host, ':') !== false
                ? '[' . $config->public_host . ']'
                : $config->public_host;
            return $host . ':' . (int)$config->public_port;
        } catch (InvalidArgumentException $exception) {
            return 'VLESS REALITY 配置无效';
        }
    }

    private static function userIsActive(User $user)
    {
        return (int)$user->enable === 1
            && strtotime($user->expire_in) > time()
            && (float)$user->transfer_enable > (float)$user->u + (float)$user->d;
    }

    private static function validateConfig(array $config)
    {
        self::validateHost($config['public_host'], '公开地址');
        if ((int)$config['public_port'] < 1 || (int)$config['public_port'] > 65535) {
            throw new InvalidArgumentException('VLESS REALITY 公开端口必须在 1 到 65535 之间');
        }
        self::validateHostname($config['server_name'], 'SNI');
        self::validateTarget($config['target']);
        if (preg_match('/^[A-Za-z0-9_-]{43}$/', $config['reality_public_key']) !== 1
            || strlen(self::decodeBase64Url($config['reality_public_key'])) !== 32
        ) {
            throw new InvalidArgumentException('REALITY 公钥必须是 32 字节无填充 Base64URL');
        }
        if (strlen($config['short_id']) > 16
            || strlen($config['short_id']) % 2 !== 0
            || preg_match('/^[0-9a-f]*$/', $config['short_id']) !== 1
        ) {
            throw new InvalidArgumentException('REALITY short ID 必须是不超过 16 位的偶数长度十六进制');
        }
        if ($config['fingerprint'] !== 'chrome'
            || $config['flow'] !== 'xtls-rprx-vision'
            || $config['transport'] !== 'raw'
        ) {
            throw new InvalidArgumentException('首版只支持 chrome + Vision + RAW/TCP');
        }
        if (strlen($config['min_client_version']) > 32
            || preg_match('/^[0-9.]*$/', $config['min_client_version']) !== 1
        ) {
            throw new InvalidArgumentException('最低客户端版本只能包含数字和点，且不得超过 32 字节');
        }
    }

    private static function validateTarget($target)
    {
        if (preg_match('/^\[([^]]+)\]:(\d{1,5})$/', $target, $match) === 1) {
            $host = $match[1];
            $port = (int)$match[2];
        } else {
            $position = strrpos($target, ':');
            if ($position === false) {
                throw new InvalidArgumentException('REALITY target 必须是 host:port');
            }
            $host = substr($target, 0, $position);
            $portText = substr($target, $position + 1);
            if ($portText === '' || !ctype_digit($portText)) {
                throw new InvalidArgumentException('REALITY target 端口无效');
            }
            $port = (int)$portText;
        }
        self::validateHost($host, 'REALITY target');
        if ($port < 1 || $port > 65535) {
            throw new InvalidArgumentException('REALITY target 端口无效');
        }
    }

    private static function validateHost($host, $label)
    {
        if (filter_var($host, FILTER_VALIDATE_IP) !== false) {
            return;
        }
        self::validateHostname($host, $label);
    }

    private static function validateHostname($host, $label)
    {
        if ($host === '' || strlen($host) > 253 || preg_match(
            '/^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)*[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/',
            $host
        ) !== 1) {
            throw new InvalidArgumentException($label . ' 无效');
        }
    }

    private static function decodeBase64Url($value)
    {
        $padding = (4 - strlen($value) % 4) % 4;
        $decoded = base64_decode(strtr($value, '-_', '+/') . str_repeat('=', $padding), true);
        return $decoded === false ? '' : $decoded;
    }
}
