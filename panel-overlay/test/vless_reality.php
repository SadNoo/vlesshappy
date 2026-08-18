<?php

namespace Illuminate\Database\Eloquent {
    class Model
    {
        protected $attributes = array();
        public static $records = array();

        public function __get($key)
        {
            return isset($this->attributes[$key]) ? $this->attributes[$key] : null;
        }

        public function __set($key, $value)
        {
            $this->attributes[$key] = $value;
        }

        public static function find($id)
        {
            $class = get_called_class();
            return isset(self::$records[$class][$id]) ? self::$records[$class][$id] : null;
        }
    }
}

namespace {
    require dirname(__DIR__, 3) . '/app/Models/Model.php';
    require dirname(__DIR__, 3) . '/app/Models/User.php';
    require dirname(__DIR__, 3) . '/app/Models/Node.php';
    require dirname(__DIR__) . '/app/Models/VlessRealityNodeConfig.php';
    require dirname(__DIR__) . '/app/Services/VlessReality.php';

    use App\Models\Node;
    use App\Models\User;
    use App\Models\VlessRealityNodeConfig;
    use App\Services\VlessReality;
    use Illuminate\Database\Eloquent\Model as EloquentModel;

    class FixtureUser extends User
    {
        public function getUuid()
        {
            return '9af01be3-7b93-39b9-8fa0-40f7ad54eb71';
        }
    }

    $user = new FixtureUser();
    $user->id = 42;
    $user->enable = 1;
    $user->expire_in = '2099-12-31 23:59:59';
    $user->transfer_enable = 1000;
    $user->u = 10;
    $user->d = 20;

    $node = new Node();
    $node->id = 15;
    $node->name = 'VLESS Test';
    $node->sort = 15;
    $node->type = 1;

    $config = new VlessRealityNodeConfig();
    $config->node_id = 15;
    $config->public_host = '2001:db8::15';
    $config->public_port = 443;
    $config->server_name = 'www.example.com';
    $config->target = 'www.example.com:443';
    $config->reality_public_key = 'jUOUBrUHcUWzzDhp4L-6l3OwfjUhOajm8Y6yL6jU1zA';
    $config->short_id = '0123456789abcdef';
    $config->fingerprint = 'chrome';
    $config->flow = 'xtls-rprx-vision';
    $config->transport = 'raw';
    $config->min_client_version = '';
    EloquentModel::$records[VlessRealityNodeConfig::class][15] = $config;

    $link = VlessReality::link($user, $node);
    assertContains('vless://9af01be3-7b93-39b9-8fa0-40f7ad54eb71@[2001:db8::15]:443?', $link, 'IPv6 URI');
    assertContains('security=reality', $link, 'REALITY URI');
    assertContains('flow=xtls-rprx-vision', $link, 'Vision URI');
    assertContains('pbk=jUOUBrUHcUWzzDhp4L-6l3OwfjUhOajm8Y6yL6jU1zA', $link, 'public key URI');

    $proxy = VlessReality::mihomoProxy($user, $node);
    assertSame('vless', $proxy['type'], 'Mihomo type');
    assertSame('xudp', $proxy['packet-encoding'], 'Mihomo XUDP');
    assertSame(true, $proxy['udp'], 'Mihomo UDP');
    assertSame($config->reality_public_key, $proxy['reality-opts']['public-key'], 'Mihomo public key');
    assertSame(base64_encode($link . "\n"), VlessReality::subscription($user, array($node)), 'subscription');

	$badRequest = new class {
		public function getParam($name)
		{
			return $name === 'vless_public_port' ? '443junk' : '';
		}
	};
	$rejected = false;
	try {
		VlessReality::requestConfig($badRequest);
	} catch (\InvalidArgumentException $exception) {
		$rejected = true;
	}
	assertSame(true, $rejected, 'strict public port');

    $user->enable = 0;
    assertSame('', VlessReality::subscription($user, array($node)), 'disabled user');

    echo "VLESS REALITY panel tests passed\n";

    function assertSame($expected, $actual, $label)
    {
        if ($expected !== $actual) {
            fail($label . ': expected ' . var_export($expected, true) . ', got ' . var_export($actual, true));
        }
    }

    function assertContains($needle, $actual, $label)
    {
        if (strpos($actual, $needle) === false) {
            fail($label . ': missing ' . $needle);
        }
    }

    function fail($message)
    {
        fwrite(STDERR, "FAIL: " . $message . "\n");
        exit(1);
    }
}
