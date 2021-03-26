Pod::Spec.new do |spec|
  spec.name         = 'Ghyk'
  spec.version      = '{{.Version}}'
  spec.license      = { :type => 'GNU Lesser General Public License, Version 3.0' }
  spec.homepage     = 'https://github.com/hayekchain/go-hayekchain'
  spec.authors      = { {{range .Contributors}}
		'{{.Name}}' => '{{.Email}}',{{end}}
	}
  spec.summary      = 'iOS HayekChain Client'
  spec.source       = { :git => 'https://github.com/hayekchain/go-hayekchain.git', :commit => '{{.Commit}}' }

	spec.platform = :ios
  spec.ios.deployment_target  = '9.0'
	spec.ios.vendored_frameworks = 'Frameworks/Ghyk.framework'

	spec.prepare_command = <<-CMD
    curl https://ghykstore.blob.core.windows.net/builds/{{.Archive}}.tar.gz | tar -xvz
    mkdir Frameworks
    mv {{.Archive}}/Ghyk.framework Frameworks
    rm -rf {{.Archive}}
  CMD
end
